//go:build linux

package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"github.com/brickingsoft/rio/pkg/kernel"
	"github.com/brickingsoft/rio/pkg/sys"
	"io"
	"net"
	"os"
	"sync/atomic"
	"syscall"
	"time"
)

func ListenTCP(network string, addr *net.TCPAddr) (*TCPListener, error) {
	config := ListenConfig{
		KeepAliveConfig: net.KeepAliveConfig{Enable: true},
		UseSendZC:       defaultUseSendZC.Load(),
	}
	ctx := context.Background()
	return config.ListenTCP(ctx, network, addr)
}

func (lc *ListenConfig) ListenTCP(ctx context.Context, network string, addr *net.TCPAddr) (*TCPListener, error) {
	// vortex
	vortex, vortexErr := getCenterVortex()
	if vortexErr != nil {
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: vortexErr}
	}
	// fd
	switch network {
	case "tcp", "tcp4", "tcp6":
	default:
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: net.UnknownNetworkError(network)}
	}
	if addr == nil {
		addr = &net.TCPAddr{}
	}
	var control ctrlCtxFn = nil
	if lc.Control != nil {
		control = func(ctx context.Context, network string, address string, raw syscall.RawConn) error {
			return lc.Control(network, address, raw)
		}
	}
	fd, fdErr := newTCPListenerFd(ctx, network, addr, lc.FastOpen, lc.MultipathTCP, control)
	if fdErr != nil {
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: fdErr}
	}

	// ctx
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)

	// sendzc
	useSendZC := lc.UseSendZC
	if useSendZC {
		useSendZC = aio.CheckSendZCEnable()
	}
	// ln
	ln := &TCPListener{
		ctx:             ctx,
		cancel:          cancel,
		fd:              fd,
		vortex:          vortex,
		useSendZC:       useSendZC,
		keepAlive:       lc.KeepAlive,
		keepAliveConfig: lc.KeepAliveConfig,
	}
	return ln, nil
}

type TCPListener struct {
	ctx             context.Context
	cancel          context.CancelFunc
	fd              *sys.Fd
	vortex          *aio.Vortex
	useSendZC       bool
	keepAlive       time.Duration
	keepAliveConfig net.KeepAliveConfig
	deadline        time.Time
}

func (ln *TCPListener) Accept() (net.Conn, error) {
	return ln.AcceptTCP()
}

func (ln *TCPListener) AcceptTCP() (tc *TCPConn, err error) {
	if !ln.ok() {
		return nil, syscall.EINVAL
	}
	ctx := ln.ctx
	var cancel context.CancelFunc = func() {}
	if deadline := ln.deadline; !deadline.IsZero() {
		if deadline.Before(time.Now()) {
			return nil, aio.Timeout
		}
		ctx, cancel = context.WithDeadline(ctx, deadline)
	}
	defer cancel()

	fd := ln.fd.Socket()
	vortex := ln.vortex
	addr := &syscall.RawSockaddrAny{}
	addrLen := syscall.SizeofSockaddrAny
	future := vortex.PrepareAccept(ctx, fd, addr, addrLen)
	accepted, acceptErr := future.Await(ctx)
	if acceptErr != nil {
		err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: acceptErr}
		return
	}

	cfd := sys.NewFd(ln.fd.Net(), accepted, ln.fd.Family(), ln.fd.SocketType())
	// local addr
	if err = cfd.LoadLocalAddr(); err != nil {
		_ = cfd.Close()
		err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
		return
	}
	// remote addr
	sa, saErr := sys.RawSockaddrAnyToSockaddr(addr)
	if saErr != nil {
		if err = cfd.LoadRemoteAddr(); err != nil {
			_ = cfd.Close()
			err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
			return
		}
	}
	localAddr := sys.SockaddrToAddr(ln.fd.Net(), sa)
	cfd.SetRemoteAddr(localAddr)
	// side
	side, sideErr := getSideVortex()
	if sideErr != nil {
		_ = cfd.Close()
		err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: sideErr}
		return
	}
	// tcp conn
	cc, cancel := context.WithCancel(ctx)
	tc = &TCPConn{
		conn{
			ctx:          cc,
			cancel:       cancel,
			fd:           cfd,
			useZC:        ln.useSendZC,
			vortex:       side,
			readTimeout:  atomic.Int64{},
			writeTimeout: atomic.Int64{},
		},
	}
	// no delay
	_ = tc.SetNoDelay(true)
	// keepalive
	keepAliveConfig := ln.keepAliveConfig
	if !keepAliveConfig.Enable && ln.keepAlive >= 0 {
		keepAliveConfig = net.KeepAliveConfig{
			Enable: true,
			Idle:   ln.keepAlive,
		}
	}
	if keepAliveConfig.Enable {
		_ = tc.SetKeepAliveConfig(keepAliveConfig)
	}
	return
}

func (ln *TCPListener) Close() error {
	if !ln.ok() {
		return syscall.EINVAL
	}
	defer func() {
		_ = UnpinVortexes()
	}()
	defer ln.cancel()

	if err := ln.fd.Close(); err != nil {
		return &net.OpError{Op: "close", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
	}
	return nil
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

func (ln *TCPListener) SetDeadline(t time.Time) error {
	if !ln.ok() {
		return syscall.EINVAL
	}
	ln.deadline = t
	return nil
}

func (ln *TCPListener) ok() bool { return ln != nil && ln.fd != nil }

func (ln *TCPListener) checkUseSendZC() {
	if ln.useSendZC {
		ver, verErr := kernel.GetKernelVersion()
		if verErr != nil {
			ln.useSendZC = false
			return
		}
		target := kernel.Version{
			Kernel: ver.Kernel,
			Major:  6,
			Minor:  0,
			Flavor: ver.Flavor,
		}
		if kernel.CompareKernelVersion(*ver, target) < 0 {
			ln.useSendZC = false
		}
	}
}

func newTCPListenerFd(ctx context.Context, network string, addr *net.TCPAddr, fastOpen int, multipathTCP bool, control ctrlCtxFn) (fd *sys.Fd, err error) {
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
	fd = sys.NewFd(network, sock, family, syscall.SOCK_STREAM)
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
	// fast open
	if err = fd.AllowFastOpen(fastOpen); err != nil {
		_ = fd.Close()
		return
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

func (c *TCPConn) SyscallConn() (syscall.RawConn, error) {
	if !c.ok() {
		return nil, syscall.EINVAL
	}
	return newRawConn(c.fd), nil
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
		return 0, syscall.EFAULT
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
			return written, &net.OpError{Op: "readfrom", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: spliceErr}
		}
		return written, nil
	case 2: // sendfile(mmap+send)
		ctx := c.ctx
		fd := c.fd.Socket()
		vortex := c.vortex
		written, sendfileErr := vortex.Sendfile(ctx, fd, r, c.useZC)
		if lr != nil {
			lr.N -= written
		}
		if sendfileErr != nil {
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
		return 0, syscall.EFAULT
	}
	uc, ok := w.(*UnixConn)
	if ok && uc.fd.Net() == "unix" {
		ctx := c.ctx
		fd := c.fd.Socket()
		vortex := c.vortex
		written, spliceErr := vortex.Splice(ctx, uc.fd.Socket(), fd, 1<<63-1)
		if spliceErr != nil {
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
