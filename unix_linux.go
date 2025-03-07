//go:build linux

package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"github.com/brickingsoft/rio/pkg/sys"
	"golang.org/x/sys/unix"
	"net"
	"os"
	"reflect"
	"sync"
	"syscall"
	"time"
)

func ListenUnix(network string, addr *net.UnixAddr) (*UnixListener, error) {
	config := ListenConfig{}
	ctx := context.Background()
	return config.ListenUnix(ctx, network, addr)
}

func (lc *ListenConfig) ListenUnix(ctx context.Context, network string, addr *net.UnixAddr) (*UnixListener, error) {
	// network
	switch network {
	case "unix", "unixpacket":
	default:
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: net.UnknownNetworkError(network)}
	}
	if addr == nil {
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: errors.New("missing address")}
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
	fd, fdErr := newUnixListener(ctx, network, addr, control)
	if fdErr != nil {
		_ = aio.Release(vortex)
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: fdErr}
	}

	// sendzc
	useSendZC := lc.UseSendZC
	if useSendZC {
		useSendZC = aio.CheckSendZCEnable()
	}

	// ln
	ln := &UnixListener{
		ctx:        ctx,
		fd:         fd,
		path:       fd.LocalAddr().String(),
		unlink:     true,
		unlinkOnce: sync.Once{},
		vortex:     vortex,
		useSendZC:  useSendZC,
		deadline:   time.Time{},
	}
	return ln, nil
}

func ListenUnixgram(network string, addr *net.UnixAddr) (*UnixConn, error) {
	config := ListenConfig{}
	ctx := context.Background()
	return config.ListenUnixgram(ctx, network, addr)
}

func (lc *ListenConfig) ListenUnixgram(ctx context.Context, network string, addr *net.UnixAddr) (*UnixConn, error) {
	// network
	switch network {
	case "unixgram":
	default:
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: net.UnknownNetworkError(network)}
	}
	if addr == nil {
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: errors.New("missing address")}
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
	fd, fdErr := newUnixListener(ctx, network, addr, control)
	if fdErr != nil {
		_ = aio.Release(vortex)
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: fdErr}
	}

	// sendzc
	useSendZC := lc.UseSendZC
	useSendMsgZC := lc.UseSendZC
	if useSendZC {
		useSendZC = aio.CheckSendMsdZCEnable()
		useSendMsgZC = aio.CheckSendMsdZCEnable()
	}
	// conn
	c := &UnixConn{
		conn{
			ctx:           ctx,
			fd:            fd,
			vortex:        vortex,
			readDeadline:  time.Time{},
			writeDeadline: time.Time{},
			useZC:         useSendZC,
			pinned:        true,
		},
		useSendMsgZC,
	}
	return c, nil
}

func newUnixListener(ctx context.Context, network string, addr *net.UnixAddr, control ctrlCtxFn) (fd *sys.Fd, err error) {
	family := syscall.AF_UNIX
	sotype := 0
	switch network {
	case "unix":
		sotype = syscall.SOCK_STREAM
		break
	case "unixpacket":
		sotype = syscall.SOCK_SEQPACKET
		break
	case "unixgram":
		sotype = syscall.SOCK_DGRAM
		break
	default:
		err = net.UnknownNetworkError(network)
		return
	}
	// fd
	sock, sockErr := sys.NewSocket(family, sotype, 0)
	if sockErr != nil {
		err = sockErr
		return
	}
	fd = sys.NewFd(network, sock, family, sotype)
	// control
	if control != nil {
		raw := newRawConn(fd)
		if err = control(ctx, fd.CtrlNetwork(), addr.String(), raw); err != nil {
			_ = fd.Close()
			return
		}
	}
	// bind
	if err = fd.Bind(addr); err != nil {
		_ = fd.Close()
		return
	}
	// listen
	if sotype != syscall.SOCK_DGRAM {
		backlog := sys.MaxListenerBacklog()
		if err = syscall.Listen(sock, backlog); err != nil {
			_ = fd.Close()
			err = os.NewSyscallError("listen", err)
			return
		}
	}
	// set socket addr
	if sn, getSockNameErr := syscall.Getsockname(sock); getSockNameErr == nil {
		if sockname := sys.SockaddrToAddr(network, sn); sockname != nil {
			fd.SetLocalAddr(sockname)
		} else {
			fd.SetLocalAddr(addr)
		}
	} else {
		fd.SetLocalAddr(addr)
	}
	return
}

type UnixListener struct {
	ctx        context.Context
	fd         *sys.Fd
	path       string
	unlink     bool
	unlinkOnce sync.Once
	vortex     *aio.Vortex
	useSendZC  bool
	deadline   time.Time
}

func (ln *UnixListener) Accept() (net.Conn, error) {
	return ln.AcceptUnix()
}

func (ln *UnixListener) AcceptUnix() (c *UnixConn, err error) {
	if !ln.ok() {
		return nil, syscall.EINVAL
	}

	ctx := ln.ctx
	fd := ln.fd.Socket()
	vortex := ln.vortex
	deadline := ln.deadline

	// accept
	addr := &syscall.RawSockaddrAny{}
	addrLen := syscall.SizeofSockaddrAny
	accepted, acceptErr := vortex.Accept(ctx, fd, addr, addrLen, deadline)
	if acceptErr != nil {
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
	// unix conn
	c = &UnixConn{
		conn{
			ctx:           ctx,
			fd:            cfd,
			useZC:         ln.useSendZC,
			vortex:        vortex,
			readDeadline:  time.Time{},
			writeDeadline: time.Time{},
			pinned:        false,
		},
		false,
	}
	return
}

func (ln *UnixListener) Close() error {
	if !ln.ok() {
		return syscall.EINVAL
	}

	ln.unlinkOnce.Do(func() {
		if ln.path[0] != '@' && ln.unlink {
			_ = syscall.Unlink(ln.path)
		}
	})

	ctx := ln.ctx
	fd := ln.fd.Socket()
	vortex := ln.vortex

	if err := vortex.Close(ctx, fd); err != nil {
		_ = syscall.Close(fd)
		_ = aio.Release(vortex)
		return &net.OpError{Op: "close", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
	}
	if unpinErr := aio.Release(vortex); unpinErr != nil {
		return &net.OpError{Op: "close", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: unpinErr}
	}
	return nil
}

func (ln *UnixListener) Addr() net.Addr {
	if !ln.ok() {
		return nil
	}
	return ln.fd.LocalAddr()
}

func (ln *UnixListener) SetDeadline(t time.Time) error {
	if !ln.ok() {
		return syscall.EINVAL
	}
	ln.deadline = t
	return nil
}

func (ln *UnixListener) SetUnlinkOnClose(unlink bool) {
	ln.unlink = unlink
}

func (ln *UnixListener) File() (f *os.File, err error) {
	if !ln.ok() {
		return nil, syscall.EINVAL
	}
	f, err = ln.file()
	if err != nil {
		err = &net.OpError{Op: "file", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
	}
	return
}

func (ln *UnixListener) ok() bool { return ln != nil && ln.fd != nil }

func (ln *UnixListener) file() (*os.File, error) {
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

func (ln *UnixListener) SyscallConn() (syscall.RawConn, error) {
	if !ln.ok() {
		return nil, syscall.EINVAL
	}
	return newRawConn(ln.fd), nil
}

type UnixConn struct {
	conn
	useMsgZC bool
}

func (c *UnixConn) ReadFrom(b []byte) (int, net.Addr, error) {
	return c.ReadFromUnix(b)
}

func (c *UnixConn) ReadFromUnix(b []byte) (n int, addr *net.UnixAddr, err error) {
	if !c.ok() {
		return 0, nil, syscall.EINVAL
	}
	if len(b) == 0 {
		return 0, nil, &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}

	ctx := c.ctx
	fd := c.fd.Socket()
	vortex := c.vortex

	rsa := &syscall.RawSockaddrAny{}
	rsaLen := syscall.SizeofSockaddrAny

	deadline := c.deadline(ctx, c.readDeadline)

	n, err = vortex.ReceiveFrom(ctx, fd, b, rsa, rsaLen, deadline)
	if err != nil {
		err = &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
		return
	}

	sa, saErr := sys.RawSockaddrAnyToSockaddr(rsa)
	if saErr != nil {
		err = &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: saErr}
		return
	}
	a := sys.SockaddrToAddr(c.fd.Net(), sa)
	ok := false
	addr, ok = a.(*net.UnixAddr)
	if !ok {
		err = &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: errors.New("wrong address type")}
		return
	}
	return
}

func (c *UnixConn) ReadMsgUnix(b []byte, oob []byte) (n, oobn, flags int, addr *net.UnixAddr, err error) {
	if !c.ok() {
		return 0, 0, 0, nil, syscall.EINVAL
	}
	bLen := len(b)
	oobLen := len(oob)
	if bLen == 0 && oobLen == 0 {
		return 0, 0, 0, nil, &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}

	ctx := c.ctx
	fd := c.fd.Socket()
	vortex := c.vortex

	rsa := &syscall.RawSockaddrAny{}
	rsaLen := syscall.SizeofSockaddrAny

	deadline := c.deadline(ctx, c.readDeadline)

	n, oobn, flags, err = vortex.ReceiveMsg(ctx, fd, b, oob, rsa, rsaLen, unix.MSG_CMSG_CLOEXEC, deadline)
	if err != nil {
		err = &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
		return
	}

	sa, saErr := sys.RawSockaddrAnyToSockaddr(rsa)
	if saErr != nil {
		err = &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: saErr}
		return
	}
	a := sys.SockaddrToAddr(c.fd.Net(), sa)
	ok := false
	addr, ok = a.(*net.UnixAddr)
	if !ok {
		err = &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: net.InvalidAddrError("wrong address type")}
		return
	}
	return
}

func (c *UnixConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	if !c.ok() {
		return 0, syscall.EINVAL
	}
	if len(b) == 0 || reflect.ValueOf(addr).IsNil() {
		return 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}
	uAddr, isUnixAddr := addr.(*net.UnixAddr)
	if !isUnixAddr {
		return 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}
	if uAddr.Net != c.fd.Net() {
		return 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}
	sa := &syscall.SockaddrUnix{Name: uAddr.Name}
	return c.writeTo(b, sa)
}

func (c *UnixConn) WriteToUnix(b []byte, addr *net.UnixAddr) (int, error) {
	return c.WriteTo(b, addr)
}

func (c *UnixConn) writeTo(b []byte, addr syscall.Sockaddr) (n int, err error) {
	rsa, rsaLen, rsaErr := sys.SockaddrToRawSockaddrAny(addr)
	if rsaErr != nil {
		return 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: rsaErr}
	}

	ctx := c.ctx
	fd := c.fd.Socket()
	vortex := c.vortex

	deadline := c.deadline(ctx, c.writeDeadline)

	if c.useMsgZC {
		n, err = vortex.SendToZC(ctx, fd, b, rsa, int(rsaLen), deadline)
	} else {
		n, err = vortex.SendTo(ctx, fd, b, rsa, int(rsaLen), deadline)
	}
	if err != nil {
		err = &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
		return
	}
	return
}

func (c *UnixConn) WriteMsgUnix(b []byte, oob []byte, addr *net.UnixAddr) (n int, oobn int, err error) {
	if !c.ok() {
		return 0, 0, syscall.EINVAL
	}
	if len(b) == 0 && len(oob) == 0 {
		return 0, 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}
	if addr == nil {
		return 0, 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}
	if addr.Net != c.fd.Net() {
		return 0, 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}
	sa := &syscall.SockaddrUnix{Name: addr.Name}
	rsa, rsaLen, rsaErr := sys.SockaddrToRawSockaddrAny(sa)
	if rsaErr != nil {
		err = &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: rsaErr}
		return
	}

	if len(b) == 0 && c.fd.SocketType() != syscall.SOCK_DGRAM {
		b = []byte{0}
	}

	ctx := c.ctx
	fd := c.fd.Socket()
	vortex := c.vortex

	deadline := c.deadline(ctx, c.writeDeadline)

	if c.useMsgZC {
		n, oobn, err = vortex.SendMsgZC(ctx, fd, b, oob, rsa, int(rsaLen), deadline)
	} else {
		n, oobn, err = vortex.SendMsg(ctx, fd, b, oob, rsa, int(rsaLen), deadline)
	}
	if err != nil {
		err = &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
		return
	}
	return
}
