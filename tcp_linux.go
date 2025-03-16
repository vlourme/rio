//go:build linux

package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"github.com/brickingsoft/rio/pkg/sys"
	"io"
	"net"
	"os"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
)

func ListenTCP(network string, addr *net.TCPAddr) (*TCPListener, error) {
	config := ListenConfig{
		Control:         nil,
		KeepAlive:       0,
		KeepAliveConfig: net.KeepAliveConfig{Enable: true},
		MultipathTCP:    false,
		FastOpen:        true,
		QuickAck:        true,
		ReusePort:       true,
	}
	ctx := context.Background()
	return config.ListenTCP(ctx, network, addr)
}

func (lc *ListenConfig) ListenTCP(ctx context.Context, network string, addr *net.TCPAddr) (*TCPListener, error) {
	// check multishot accept
	if lc.MultishotAccept {
		lc.MultishotAccept = aio.CheckMultishotAcceptEnable()
	}
	// network
	switch network {
	case "tcp", "tcp4", "tcp6":
	default:
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: net.UnknownNetworkError(network)}
	}
	if addr == nil {
		addr = &net.TCPAddr{}
	}
	// vortex
	vortex, vortexErr := aio.Acquire()
	if vortexErr != nil {
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: vortexErr}
	}
	// fd
	var control ctrlCtxFn = nil
	if lc.Control != nil {
		control = func(ctx context.Context, network string, address string, raw syscall.RawConn) error {
			return lc.Control(network, address, raw)
		}
	}
	fd, fdErr := newTCPListenerFd(ctx, network, addr, lc.FastOpen, lc.QuickAck, lc.MultipathTCP, lc.ReusePort, control)
	if fdErr != nil {
		_ = aio.Release(vortex)
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: fdErr}
	}

	// install fixed fd
	autoFixedFdInstall := lc.AutoFixedFdInstall
	fileIndex := -1
	sqeFlags := uint8(0)
	if vortex.RegisterFixedFdEnabled() {
		sock := fd.Socket()
		file, regErr := vortex.RegisterFixedFd(ctx, sock)
		if regErr == nil {
			fileIndex = file
			sqeFlags = iouring.SQEFixedFile
		} else {
			if !errors.Is(regErr, aio.ErrFixedFileUnavailable) {
				_ = fd.Close()
				_ = aio.Release(vortex)
				return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: regErr}
			}
			autoFixedFdInstall = false
		}
	} else {
		autoFixedFdInstall = false
	}

	// send zc
	useSendZC := false
	if lc.UseSendZC {
		useSendZC = aio.CheckSendZCEnable()
	}

	// ln
	cc, cancel := context.WithCancel(ctx)
	ln := &TCPListener{
		ctx:                cc,
		cancel:             cancel,
		fd:                 fd,
		autoFixedFdInstall: autoFixedFdInstall,
		fdFixed:            fileIndex != -1,
		fileIndex:          fileIndex,
		sqeFlags:           sqeFlags,
		vortex:             vortex,
		acceptFuture:       nil,
		multipathTCP:       lc.MultipathTCP,
		keepAlive:          lc.KeepAlive,
		keepAliveConfig:    lc.KeepAliveConfig,
		useSendZC:          useSendZC,
		useMultishotAccept: lc.MultishotAccept,
	}
	// prepare multishot accept
	if ln.useMultishotAccept {
		if err := ln.prepareMultishotAccepting(); err != nil {
			_ = ln.Close()
			return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: err}
		}
	}

	return ln, nil
}

type TCPListener struct {
	ctx                context.Context
	cancel             context.CancelFunc
	fd                 *sys.Fd
	autoFixedFdInstall bool
	fdFixed            bool
	fileIndex          int
	sqeFlags           uint8
	vortex             *aio.Vortex
	acceptFuture       *aio.Future
	multipathTCP       bool
	keepAlive          time.Duration
	keepAliveConfig    net.KeepAliveConfig
	useSendZC          bool
	useMultishotAccept bool
}

func (ln *TCPListener) Accept() (net.Conn, error) {
	return ln.AcceptTCP()
}

func (ln *TCPListener) AcceptTCP() (tc *TCPConn, err error) {
	if !ln.ok() {
		return nil, syscall.EINVAL
	}

	if ln.useMultishotAccept {
		tc, err = ln.acceptMultishot()
	} else {
		tc, err = ln.acceptOneshot()
	}
	return
}

func (ln *TCPListener) acceptOneshot() (tc *TCPConn, err error) {
	ctx := ln.ctx
	fd := 0
	if ln.fdFixed {
		fd = ln.fileIndex
	} else {
		fd = ln.fd.Socket()
	}
	vortex := ln.vortex

	// accept
	addr := &syscall.RawSockaddrAny{}
	addrLen := syscall.SizeofSockaddrAny
	accepted, acceptErr := vortex.Accept(ctx, fd, addr, addrLen, ln.sqeFlags)
	if acceptErr != nil {
		if errors.Is(acceptErr, context.Canceled) {
			acceptErr = net.ErrClosed
		}
		err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: acceptErr}
		return
	}
	// fd
	cfd := sys.NewFd(ln.fd.Net(), accepted, ln.fd.Family(), ln.fd.SocketType())
	// local addr
	if err = cfd.LoadLocalAddr(); err != nil {
		_ = cfd.Close()
		err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
		return
	}
	// remote addr
	if sa, saErr := sys.RawSockaddrAnyToSockaddr(addr); saErr == nil {
		remoteAddr := sys.SockaddrToAddr(ln.fd.Net(), sa)
		cfd.SetRemoteAddr(remoteAddr)
	} else {
		if err = cfd.LoadRemoteAddr(); err != nil {
			_ = cfd.Close()
			err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
			return
		}
	}
	// set non blocking
	if err = cfd.SetNonblocking(true); err != nil {
		_ = cfd.Close()
		err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
		return
	}
	// no delay
	_ = cfd.SetNoDelay(true)
	// keepalive
	keepAliveConfig := ln.keepAliveConfig
	if !keepAliveConfig.Enable && ln.keepAlive >= 0 {
		keepAliveConfig = net.KeepAliveConfig{
			Enable: true,
			Idle:   ln.keepAlive,
		}
	}
	if keepAliveConfig.Enable {
		_ = cfd.SetKeepAliveConfig(keepAliveConfig)
	}
	// fixed fd
	fileIndex := -1
	sqeFlags := uint8(0)
	if ln.autoFixedFdInstall {
		sock := cfd.Socket()
		file, regErr := vortex.RegisterFixedFd(ctx, sock)
		if regErr == nil {
			fileIndex = file
			sqeFlags = iouring.SQEFixedFile
		} else {
			if !errors.Is(regErr, aio.ErrFixedFileUnavailable) {
				_ = cfd.Close()
				err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: regErr}
				return
			}
		}
	}

	// conn
	cc, cancel := context.WithCancel(ctx)
	tc = &TCPConn{
		conn{
			ctx:           cc,
			cancel:        cancel,
			fd:            cfd,
			fdFixed:       fileIndex != -1,
			fileIndex:     fileIndex,
			sqeFlags:      sqeFlags,
			vortex:        vortex,
			readDeadline:  time.Time{},
			writeDeadline: time.Time{},
			readBuffer:    atomic.Int64{},
			writeBuffer:   atomic.Int64{},
			pinned:        false,
			useSendZC:     ln.useSendZC,
		},
	}
	return
}

func (ln *TCPListener) acceptMultishot() (tc *TCPConn, err error) {
	ctx := ln.ctx

	accepted, _, acceptErr := ln.acceptFuture.Await(ctx)
	if acceptErr != nil {
		if errors.Is(acceptErr, context.Canceled) {
			acceptErr = net.ErrClosed
		}
		err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: acceptErr}
		return
	}
	// fd
	cfd := sys.NewFd(ln.fd.Net(), accepted, ln.fd.Family(), ln.fd.SocketType())
	// local addr
	if err = cfd.LoadLocalAddr(); err != nil {
		_ = cfd.Close()
		err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
		return
	}
	// remote addr
	if err = cfd.LoadRemoteAddr(); err != nil {
		_ = cfd.Close()
		err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
		return
	}
	// set non blocking
	if err = cfd.SetNonblocking(true); err != nil {
		_ = cfd.Close()
		err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
		return
	}
	// no delay
	_ = cfd.SetNoDelay(true)
	// keepalive
	keepAliveConfig := ln.keepAliveConfig
	if !keepAliveConfig.Enable && ln.keepAlive >= 0 {
		keepAliveConfig = net.KeepAliveConfig{
			Enable: true,
			Idle:   ln.keepAlive,
		}
	}
	if keepAliveConfig.Enable {
		_ = cfd.SetKeepAliveConfig(keepAliveConfig)
	}

	// fixed fd
	fileIndex := -1
	sqeFlags := uint8(0)
	if ln.autoFixedFdInstall {
		sock := cfd.Socket()
		file, regErr := ln.vortex.RegisterFixedFd(ctx, sock)
		if regErr == nil {
			fileIndex = file
			sqeFlags = iouring.SQEFixedFile
		} else {
			if !errors.Is(regErr, aio.ErrFixedFileUnavailable) {
				_ = cfd.Close()
				err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: regErr}
				return
			}
		}
	}

	// tcp conn
	cc, cancel := context.WithCancel(ctx)
	tc = &TCPConn{
		conn{
			ctx:           cc,
			cancel:        cancel,
			fd:            cfd,
			fdFixed:       fileIndex != -1,
			fileIndex:     fileIndex,
			sqeFlags:      sqeFlags,
			vortex:        ln.vortex,
			readDeadline:  time.Time{},
			writeDeadline: time.Time{},
			readBuffer:    atomic.Int64{},
			writeBuffer:   atomic.Int64{},
			pinned:        false,
			useSendZC:     ln.useSendZC,
		},
	}
	return
}

func (ln *TCPListener) prepareMultishotAccepting() (err error) {
	vortex := ln.vortex

	fd := 0
	if ln.fdFixed {
		fd = ln.fileIndex
	} else {
		fd = ln.fd.Socket()
	}

	addr := &syscall.RawSockaddrAny{}
	addrLen := syscall.SizeofSockaddrAny
	backlog := sys.MaxListenerBacklog()
	if backlog < 1024 {
		backlog = 1024
	}
	future := vortex.AcceptMultishotAsync(fd, addr, addrLen, backlog, ln.sqeFlags)
	ln.acceptFuture = &future
	return
}

func (ln *TCPListener) Close() error {
	if !ln.ok() {
		return syscall.EINVAL
	}

	defer ln.cancel()

	ctx := ln.ctx
	vortex := ln.vortex

	if ln.useMultishotAccept {
		op := ln.acceptFuture.Operation()
		vortex.Cancel(op)
	}

	var err error
	if ln.fdFixed {
		err = vortex.CloseDirect(ctx, ln.fileIndex)
		_ = vortex.UnregisterFixedFd(ln.fileIndex)
	} else {
		fd := ln.fd.Socket()
		err = vortex.Close(ctx, fd)
	}
	if err != nil {
		fd := ln.fd.Socket()
		_ = syscall.Close(fd)
		_ = aio.Release(vortex)
		return &net.OpError{Op: "close", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
	}

	if unpinErr := aio.Release(vortex); unpinErr != nil {
		return &net.OpError{Op: "close", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: unpinErr}
	}
	return nil
}

func (ln *TCPListener) InstallFixedFd() error {
	if !ln.ok() {
		return syscall.EINVAL
	}
	if ln.fdFixed {
		return nil
	}
	ctx := ln.ctx
	vortex := ln.vortex

	sock := ln.fd.Socket()

	file, regErr := vortex.RegisterFixedFd(ctx, sock)
	if regErr != nil {
		return &net.OpError{Op: "install_fixed_file", Net: ln.fd.Net(), Source: ln.fd.LocalAddr(), Addr: ln.fd.RemoteAddr(), Err: regErr}
	}

	ln.fdFixed = true
	ln.fileIndex = file
	ln.sqeFlags |= iouring.SQEFixedFile
	return nil
}

func (ln *TCPListener) FixedFdInstalled() bool {
	return ln.fdFixed
}

func (ln *TCPListener) Addr() net.Addr {
	if !ln.ok() {
		return nil
	}
	return ln.fd.LocalAddr()
}

func (ln *TCPListener) SyscallConn() (syscall.RawConn, error) {
	if !ln.ok() {
		return nil, syscall.EINVAL
	}
	return newRawConn(ln.fd), nil
}

func (ln *TCPListener) File() (f *os.File, err error) {
	if !ln.ok() {
		return nil, syscall.EINVAL
	}
	f, err = ln.file()
	if err != nil {
		return nil, &net.OpError{Op: "file", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
	}
	return
}

func (ln *TCPListener) file() (*os.File, error) {
	ns, call, err := ln.fd.Dup()
	if err != nil {
		if call != "" {
			err = os.NewSyscallError(call, err)
		}
		return nil, err
	}
	f := os.NewFile(uintptr(ns), ln.fd.Name())
	return f, nil
}

func (ln *TCPListener) ok() bool { return ln != nil && ln.fd != nil }

func newTCPListenerFd(ctx context.Context, network string, addr *net.TCPAddr, fastOpen bool, quickAck bool, multipathTCP bool, reusePort bool, control ctrlCtxFn) (fd *sys.Fd, err error) {
	resolveAddr, family, ipv6only, addrErr := sys.ResolveAddr(network, addr.String())
	if addrErr != nil {
		err = addrErr
		return
	}
	// proto
	proto := syscall.IPPROTO_TCP
	if multipathTCP {
		if mp, ok := sys.TryGetMultipathTCPProto(); ok {
			proto = mp
		}
	}
	// fd
	sock, sockErr := sys.NewSocket(family, syscall.SOCK_STREAM, proto)
	if sockErr != nil {
		err = sockErr
		return
	}
	fd = sys.NewFd(network, sock, family, syscall.SOCK_STREAM) //

	// ipv6
	if ipv6only {
		if err = fd.SetIpv6only(true); err != nil {
			_ = fd.Close()
			return
		}
	}
	// reuse addr
	if err = fd.AllowReuseAddr(); err != nil {
		_ = fd.Close()
		return
	}
	// reuse port
	if reusePort {
		if err = fd.AllowReusePort(addr.Port); err != nil {
			_ = fd.Close()
			return
		}
		// cbpf (dep reuse port)
		filter := sys.NewFilter(uint32(runtime.NumCPU()))
		if err = filter.ApplyTo(fd.Socket()); err != nil {
			_ = fd.Close()
			return
		}
	}
	// fast open
	if fastOpen {
		if err = fd.AllowFastOpen(fastOpen); err != nil {
			_ = fd.Close()
			return
		}
	}
	// quick ack
	if quickAck {
		if err = fd.AllowQuickAck(quickAck); err != nil {
			_ = fd.Close()
			return
		}
	}
	// defer accept
	if err = syscall.SetsockoptInt(sock, syscall.IPPROTO_TCP, syscall.TCP_DEFER_ACCEPT, 1); err != nil {
		_ = fd.Close()
		err = os.NewSyscallError("setsockopt", err)
		return
	}
	// control
	if control != nil {
		raw := newRawConn(fd)
		if err = control(ctx, fd.CtrlNetwork(), addr.String(), raw); err != nil {
			_ = fd.Close()
			return
		}
	}
	// bind
	if err = fd.Bind(resolveAddr); err != nil {
		_ = fd.Close()
		return
	}
	// listen
	backlog := sys.MaxListenerBacklog()
	if err = syscall.Listen(sock, backlog); err != nil {
		_ = fd.Close()
		err = os.NewSyscallError("listen", err)
		return
	}
	// set socket addr
	if sn, getSockNameErr := syscall.Getsockname(sock); getSockNameErr == nil {
		if sockname := sys.SockaddrToAddr(network, sn); sockname != nil {
			fd.SetLocalAddr(sockname)
		} else {
			fd.SetLocalAddr(resolveAddr)
		}
	} else {
		fd.SetLocalAddr(resolveAddr)
	}
	return
}

type noReadFrom struct{}

func (noReadFrom) ReadFrom(io.Reader) (int64, error) {
	panic("can't happen")
}

type tcpConnWithoutReadFrom struct {
	noReadFrom
	*TCPConn
}

func genericReadFrom(c *TCPConn, r io.Reader) (n int64, err error) {
	return io.Copy(tcpConnWithoutReadFrom{TCPConn: c}, r)
}

type noWriteTo struct{}

func (noWriteTo) WriteTo(io.Writer) (int64, error) {
	panic("can't happen")
}

type tcpConnWithoutWriteTo struct {
	noWriteTo
	*TCPConn
}

func genericWriteTo(c *TCPConn, w io.Writer) (n int64, err error) {
	// Use wrapper to hide existing w.WriteTo from io.Copy.
	return io.Copy(w, tcpConnWithoutWriteTo{TCPConn: c})
}

type TCPConn struct {
	conn
}

const (
	ReadFromFileUseMMapPolicy = int32(iota)
	ReadFromFileUseMixPolicy
)

var readFromFilePolicy atomic.Int32

func UseReadFromFilePolicy(policy int32) {
	if policy < 0 || policy > 1 {
		policy = ReadFromFileUseMMapPolicy
	}
	readFromFilePolicy.Store(policy)
}

func (c *TCPConn) ReadFrom(r io.Reader) (int64, error) {
	if !c.ok() {
		return 0, syscall.EINVAL
	}
	if r == nil {
		return 0, &net.OpError{Op: "readfrom", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}
	var remain int64 = 1<<63 - 1 // by default, copy until EOF
	lr, ok := r.(*io.LimitedReader)
	if ok {
		remain, r = lr.N, lr.R
		if remain <= 0 {
			return 0, nil
		}
	}

	sendMode := 0 // 0: copy, 1: splice, 2: mmap
	var srcFd int
	switch v := r.(type) {
	case *TCPConn:
		srcFd = v.fd.Socket()
		sendMode = 1
		break
	case tcpConnWithoutWriteTo:
		srcFd = v.fd.Socket()
		sendMode = 1
		break
	case *UnixConn:
		if v.fd.Net() != "unix" {
			break
		}
		srcFd = v.fd.Socket()
		sendMode = 1
		break
	case *os.File:
		policy := readFromFilePolicy.Load()
		if policy == ReadFromFileUseMixPolicy {
			if remain == 1<<63-1 {
				info, infoErr := v.Stat()
				if infoErr != nil {
					return 0, infoErr
				}
				remain = info.Size()
			}
			wb, _ := c.WriteBuffer()
			if remain <= int64(wb) {
				srcFd = int(v.Fd())
				sendMode = 1
			} else {
				sendMode = 2
			}
		} else {
			sendMode = 2
		}
		break
	default:
		break
	}

	switch sendMode {
	case 1: // splice
		if srcFd < 1 {
			return 0, &net.OpError{Op: "readfrom", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: errors.New("no file descriptor found in reader")}
		}
		ctx := c.ctx
		fd := c.fd.Socket()
		vortex := c.vortex
		written, spliceErr := vortex.Splice(ctx, fd, srcFd, remain)
		if lr != nil {
			lr.N -= written
		}
		if spliceErr != nil {
			if errors.Is(spliceErr, context.Canceled) {
				spliceErr = net.ErrClosed
			}
			return written, &net.OpError{Op: "readfrom", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: spliceErr}
		}
		return written, nil
	case 2: // sendfile(mmap+send)
		ctx := c.ctx
		fd := c.fd.Socket()
		vortex := c.vortex
		written, sendfileErr := vortex.Sendfile(ctx, fd, r, c.useSendZC)
		if lr != nil {
			lr.N -= written
		}
		if sendfileErr != nil {
			if errors.Is(sendfileErr, context.Canceled) {
				sendfileErr = net.ErrClosed
			}
			return written, &net.OpError{Op: "readfrom", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: sendfileErr}
		}
		return written, nil
	default: // copy
		written, readFromErr := genericReadFrom(c, r)
		if readFromErr != nil && readFromErr != io.EOF {
			return written, &net.OpError{Op: "readfrom", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: readFromErr}
		}
		return written, nil
	}
}

func (c *TCPConn) WriteTo(w io.Writer) (int64, error) {
	if !c.ok() {
		return 0, syscall.EINVAL
	}
	if w == nil {
		return 0, &net.OpError{Op: "writeto", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}
	uc, ok := w.(*UnixConn)
	if ok && uc.fd.Net() == "unix" {
		ctx := c.ctx
		fd := c.fd.Socket()
		vortex := c.vortex
		written, spliceErr := vortex.Splice(ctx, uc.fd.Socket(), fd, 1<<63-1)
		if spliceErr != nil {
			if errors.Is(spliceErr, context.Canceled) {
				spliceErr = net.ErrClosed
			}
			return written, &net.OpError{Op: "writeto", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: spliceErr}
		}
		return written, nil
	}

	// copy
	written, writeToErr := genericWriteTo(c, w)
	if writeToErr != nil && writeToErr != io.EOF {
		writeToErr = &net.OpError{Op: "writeto", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: writeToErr}
	}
	return written, writeToErr
}

func (c *TCPConn) CloseRead() error {
	if !c.ok() {
		return syscall.EINVAL
	}
	if err := c.fd.CloseRead(); err != nil {
		return &net.OpError{Op: "close", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (c *TCPConn) CloseWrite() error {
	if !c.ok() {
		return syscall.EINVAL
	}
	if err := c.fd.CloseWrite(); err != nil {
		return &net.OpError{Op: "close", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (c *TCPConn) SetLinger(sec int) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	if err := c.fd.SetLinger(sec); err != nil {
		return &net.OpError{Op: "set", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (c *TCPConn) SetNoDelay(noDelay bool) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	if err := c.fd.SetNoDelay(noDelay); err != nil {
		return &net.OpError{Op: "set", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (c *TCPConn) SetKeepAlive(keepalive bool) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	if err := c.fd.SetKeepAlive(keepalive); err != nil {
		return &net.OpError{Op: "set", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (c *TCPConn) SetKeepAlivePeriod(period time.Duration) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	if err := c.fd.SetKeepAlivePeriod(period); err != nil {
		return &net.OpError{Op: "set", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (c *TCPConn) SetKeepAliveConfig(config net.KeepAliveConfig) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	if err := c.fd.SetKeepAliveConfig(config); err != nil {
		return &net.OpError{Op: "set", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (c *TCPConn) MultipathTCP() (bool, error) {
	if !c.ok() {
		return false, syscall.EINVAL
	}
	ok := sys.IsUsingMultipathTCP(c.fd)
	return ok, nil
}
