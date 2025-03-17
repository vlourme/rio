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

// ListenTCP acts like [Listen] for TCP networks.
//
// The network must be a TCP network name; see func Dial for details.
//
// If the IP field of laddr is nil or an unspecified IP address,
// ListenTCP listens on all available unicast and anycast IP addresses
// of the local system.
// If the Port field of laddr is 0, a port number is automatically
// chosen.
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

// ListenTCP acts like [Listen] for TCP networks.
//
// The network must be a TCP network name; see func Dial for details.
//
// If the IP field of laddr is nil or an unspecified IP address,
// ListenTCP listens on all available unicast and anycast IP addresses
// of the local system.
// If the Port field of laddr is 0, a port number is automatically
// chosen.
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
	vortex := lc.Vortex
	if vortex == nil {
		var vortexErr error
		vortex, vortexErr = getVortex()
		if vortexErr != nil {
			return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: vortexErr}
		}
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
				return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: regErr}
			}
			autoFixedFdInstall = false
		}
	} else {
		autoFixedFdInstall = false
	}

	// send zc
	useSendZC := false
	if lc.SendZC {
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

// TCPListener is a TCP network listener. Clients should typically
// use variables of type [net.Listener] instead of assuming TCP.
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
	deadline           time.Time
}

// Accept implements the Accept method in the [net.Listener] interface; it
// waits for the next call and returns a generic [net.Conn].
func (ln *TCPListener) Accept() (net.Conn, error) {
	return ln.AcceptTCP()
}

// AcceptTCP accepts the next incoming call and returns the new
// connection.
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
	accepted, acceptErr := vortex.Accept(ctx, fd, addr, addrLen, ln.deadline, ln.sqeFlags)
	if acceptErr != nil {
		if aio.IsCanceled(acceptErr) {
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
			useSendZC:     ln.useSendZC,
		},
		0,
	}
	return
}

func (ln *TCPListener) acceptMultishot() (tc *TCPConn, err error) {
	ctx := ln.ctx

	accepted, _, acceptErr := ln.acceptFuture.Await(ctx)
	if acceptErr != nil {
		if aio.IsCanceled(acceptErr) {
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
			useSendZC:     ln.useSendZC,
		},
		0,
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

// Close stops listening on the TCP address.
// Already Accepted connections are not closed.
func (ln *TCPListener) Close() error {
	if !ln.ok() {
		return syscall.EINVAL
	}

	defer ln.cancel()

	ctx := ln.ctx
	vortex := ln.vortex

	fd := ln.fd.Socket()
	_ = vortex.CancelFd(ctx, fd)

	if ln.fdFixed {
		_ = vortex.CancelFixedFd(ctx, ln.fileIndex)
		_ = vortex.CloseDirect(ctx, ln.fileIndex)
		_ = vortex.UnregisterFixedFd(ln.fileIndex)
	}

	err := vortex.Close(ctx, fd)
	if err != nil {
		_ = syscall.Close(fd)
		return &net.OpError{Op: "close", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
	}
	return nil
}

// InstallFixedFd implements the InstallFixedFd method in the [FixedFd] interface.
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

// FixedFdInstalled implements the FixedFdInstalled method in the [FixedFd] interface.
func (ln *TCPListener) FixedFdInstalled() bool {
	return ln.fdFixed
}

// Addr returns the listener's network address, a [*TCPAddr].
// The Addr returned is shared by all invocations of Addr, so
// do not modify it.
func (ln *TCPListener) Addr() net.Addr {
	if !ln.ok() {
		return nil
	}
	return ln.fd.LocalAddr()
}

// SetDeadline sets the deadline associated with the listener.
// A zero time value disables the deadline.
// Only valid when not multishot accept mode.
func (ln *TCPListener) SetDeadline(t time.Time) error {
	if !ln.ok() {
		return syscall.EINVAL
	}
	ln.deadline = t
	return nil
}

// SyscallConn returns a raw network connection.
// This implements the [syscall.Conn] interface.
//
// The returned RawConn only supports calling Control. Read and
// Write return an error.
func (ln *TCPListener) SyscallConn() (syscall.RawConn, error) {
	if !ln.ok() {
		return nil, syscall.EINVAL
	}
	return newRawConn(ln.fd), nil
}

// File returns a copy of the underlying [os.File].
// It is the caller's responsibility to close f when finished.
// Closing l does not affect f, and closing f does not affect l.
//
// The returned os.File's file descriptor is different from the
// connection's. Attempting to change properties of the original
// using this duplicate may or may not have the desired effect.
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

// TCPConn is an implementation of the [net.Conn] interface for TCP network
// connections.
type TCPConn struct {
	conn
	readFromFilePolicy int32
}

const (
	ReadFromFileUseMMapPolicy = int32(iota)
	ReadFromFileUseMixPolicy
)

func (c *TCPConn) SetReadFromFilePolicy(policy int32) {
	switch policy {
	case ReadFromFileUseMMapPolicy, ReadFromFileUseMixPolicy:
		c.readFromFilePolicy = policy
		break
	default:
		break
	}
}

// ReadFrom implements the [io.ReaderFrom] ReadFrom method.
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
		if c.readFromFilePolicy == ReadFromFileUseMixPolicy {
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

// WriteTo implements the io.WriterTo WriteTo method.
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

// CloseRead shuts down the reading side of the TCP connection.
// Most callers should just use Close.
func (c *TCPConn) CloseRead() error {
	if !c.ok() {
		return syscall.EINVAL
	}
	if err := c.fd.CloseRead(); err != nil {
		return &net.OpError{Op: "close", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
	}
	return nil
}

// CloseWrite shuts down the writing side of the TCP connection.
// Most callers should just use Close.
func (c *TCPConn) CloseWrite() error {
	if !c.ok() {
		return syscall.EINVAL
	}
	if err := c.fd.CloseWrite(); err != nil {
		return &net.OpError{Op: "close", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
	}
	return nil
}

// SetLinger sets the behavior of Close on a connection which still
// has data waiting to be sent or to be acknowledged.
//
// If sec < 0 (the default), the operating system finishes sending the
// data in the background.
//
// If sec == 0, the operating system discards any unsent or
// unacknowledged data.
//
// If sec > 0, the data is sent in the background as with sec < 0.
// On some operating systems including Linux, this may cause Close to block
// until all data has been sent or discarded.
// On some operating systems after sec seconds have elapsed any remaining
// unsent data may be discarded.
func (c *TCPConn) SetLinger(sec int) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	if err := c.fd.SetLinger(sec); err != nil {
		return &net.OpError{Op: "set", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
	}
	return nil
}

// SetNoDelay controls whether the operating system should delay
// packet transmission in hopes of sending fewer packets (Nagle's
// algorithm).  The default is true (no delay), meaning that data is
// sent as soon as possible after a Write.
func (c *TCPConn) SetNoDelay(noDelay bool) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	if err := c.fd.SetNoDelay(noDelay); err != nil {
		return &net.OpError{Op: "set", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
	}
	return nil
}

// SetKeepAlive sets whether the operating system should send
// keep-alive messages on the connection.
func (c *TCPConn) SetKeepAlive(keepalive bool) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	if err := c.fd.SetKeepAlive(keepalive); err != nil {
		return &net.OpError{Op: "set", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
	}
	return nil
}

// SetKeepAlivePeriod sets the duration the connection needs to
// remain idle before TCP starts sending keepalive probes.
//
// Note that calling this method on Windows prior to Windows 10 version 1709
// will reset the KeepAliveInterval to the default system value, which is normally 1 second.
func (c *TCPConn) SetKeepAlivePeriod(period time.Duration) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	if err := c.fd.SetKeepAlivePeriod(period); err != nil {
		return &net.OpError{Op: "set", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
	}
	return nil
}

// SetKeepAliveConfig configures keep-alive messages sent by the operating system.
func (c *TCPConn) SetKeepAliveConfig(config net.KeepAliveConfig) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	if err := c.fd.SetKeepAliveConfig(config); err != nil {
		return &net.OpError{Op: "set", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
	}
	return nil
}

// MultipathTCP reports whether the ongoing connection is using MPTCP.
//
// If Multipath TCP is not supported by the host, by the other peer or
// intentionally / accidentally filtered out by a device in between, a
// fallback to TCP will be done. This method does its best to check if
// MPTCP is still being used or not.
//
// On Linux, more conditions are verified on kernels >= v5.16, improving
// the results.
func (c *TCPConn) MultipathTCP() (bool, error) {
	if !c.ok() {
		return false, syscall.EINVAL
	}
	ok := sys.IsUsingMultipathTCP(c.fd)
	return ok, nil
}
