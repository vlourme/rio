//go:build linux

package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"github.com/brickingsoft/rio/pkg/iouring/aio/sys"
	"golang.org/x/sys/unix"
	"net"
	"os"
	"reflect"
	"sync"
	"syscall"
	"time"
)

// ListenUnix acts like [Listen] for Unix networks.
//
// The network must be "unix" or "unixpacket".
func ListenUnix(network string, addr *net.UnixAddr) (*UnixListener, error) {
	config := ListenConfig{}
	ctx := context.Background()
	return config.ListenUnix(ctx, network, addr)
}

// ListenUnix acts like [Listen] for Unix networks.
//
// The network must be "unix" or "unixpacket".
func (lc *ListenConfig) ListenUnix(ctx context.Context, network string, addr *net.UnixAddr) (*UnixListener, error) {
	// check multishot accept
	if lc.MultishotAccept {
		lc.MultishotAccept = aio.CheckMultishotAcceptEnable()
	}
	// network
	sotype := 0
	switch network {
	case "unix":
		sotype = syscall.SOCK_STREAM
		break
	case "unixpacket":
		sotype = syscall.SOCK_SEQPACKET
		break
	default:
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: net.UnknownNetworkError(network)}
	}
	if addr == nil {
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: errors.New("missing address")}
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
	fd, fdErr := aio.OpenNetFd(ctx, vortex, aio.ListenMode, network, sotype, 0, addr, nil, false)
	if fdErr != nil {
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: fdErr}
	}

	// control
	if lc.Control != nil {
		control := func(ctx context.Context, network string, address string, raw syscall.RawConn) error {
			return lc.Control(network, address, raw)
		}
		raw := sys.NewRawConn(fd.RegularSocket())
		if err := control(ctx, fd.CtrlNetwork(), addr.String(), raw); err != nil {
			_ = fd.Close()
			return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: err}
		}
	}
	// bind
	if err := fd.Bind(addr); err != nil {
		_ = fd.Close()
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: err}
	}
	// listen
	backlog := sys.MaxListenerBacklog()
	if err := syscall.Listen(fd.RegularSocket(), backlog); err != nil {
		_ = fd.Close()
		err = os.NewSyscallError("listen", err)
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: err}
	}
	// set socket addr
	if sn, getSockNameErr := syscall.Getsockname(fd.RegularSocket()); getSockNameErr == nil {
		if sockname := sys.SockaddrToAddr(network, sn); sockname != nil {
			fd.SetLocalAddr(sockname)
		} else {
			fd.SetLocalAddr(addr)
		}
	} else {
		fd.SetLocalAddr(addr)
	}
	// send zc
	useSendZC := false
	if lc.SendZC {
		useSendZC = aio.CheckSendZCEnable()
	}
	// ln
	ln := &UnixListener{
		fd:                 fd,
		path:               fd.LocalAddr().String(),
		unlink:             true,
		unlinkOnce:         sync.Once{},
		vortex:             vortex,
		acceptFuture:       nil,
		useSendZC:          useSendZC,
		useMultishotAccept: lc.MultishotAccept,
		deadline:           time.Time{},
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

// ListenUnixgram acts like [ListenPacket] for Unix networks.
//
// The network must be "unixgram".
func ListenUnixgram(network string, addr *net.UnixAddr) (*UnixConn, error) {
	config := ListenConfig{}
	ctx := context.Background()
	return config.ListenUnixgram(ctx, network, addr)
}

// ListenUnixgram acts like [ListenPacket] for Unix networks.
//
// The network must be "unixgram".
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
	vortex := lc.Vortex
	if vortex == nil {
		var vortexErr error
		vortex, vortexErr = getVortex()
		if vortexErr != nil {
			return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: vortexErr}
		}
	}
	// fd
	fd, fdErr := aio.OpenNetFd(ctx, vortex, aio.ListenMode, network, syscall.SOCK_DGRAM, 0, addr, nil, false)
	if fdErr != nil {
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: fdErr}
	}

	// control
	if lc.Control != nil {
		control := func(ctx context.Context, network string, address string, raw syscall.RawConn) error {
			return lc.Control(network, address, raw)
		}
		raw := sys.NewRawConn(fd.RegularSocket())
		if err := control(ctx, fd.CtrlNetwork(), addr.String(), raw); err != nil {
			_ = fd.Close()
			return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: err}
		}
	}
	// bind
	if err := fd.Bind(addr); err != nil {
		_ = fd.Close()
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: err}
	}
	// set socket addr
	if sn, getSockNameErr := syscall.Getsockname(fd.RegularSocket()); getSockNameErr == nil {
		if sockname := sys.SockaddrToAddr(network, sn); sockname != nil {
			fd.SetLocalAddr(sockname)
		} else {
			fd.SetLocalAddr(addr)
		}
	} else {
		fd.SetLocalAddr(addr)
	}

	// send zc
	useSendZC := false
	useSendMSGZC := false
	if lc.SendZC {
		useSendZC = aio.CheckSendZCEnable()
		useSendMSGZC = aio.CheckSendMsdZCEnable()
	}
	// conn
	c := &UnixConn{
		conn{
			fd:            fd,
			readDeadline:  time.Time{},
			writeDeadline: time.Time{},
			useSendZC:     useSendZC,
		},
		useSendMSGZC,
	}
	return c, nil
}

// UnixListener is a Unix domain socket listener. Clients should
// typically use variables of type [net.Listener] instead of assuming Unix
// domain sockets.
type UnixListener struct {
	fd                 *aio.NetFd
	path               string
	unlink             bool
	unlinkOnce         sync.Once
	vortex             *aio.Vortex
	acceptFuture       *aio.AcceptFuture
	useSendZC          bool
	useMultishotAccept bool
	deadline           time.Time
}

// Accept implements the Accept method in the [net.Listener] interface.
// Returned connections will be of type [*UnixConn].
func (ln *UnixListener) Accept() (net.Conn, error) {
	return ln.AcceptUnix()
}

// AcceptUnix accepts the next incoming call and returns the new
// connection.
func (ln *UnixListener) AcceptUnix() (c *UnixConn, err error) {
	if !ln.ok() {
		return nil, syscall.EINVAL
	}

	if ln.useMultishotAccept {
		c, err = ln.acceptMultishot()
	} else {
		c, err = ln.acceptOneshot()
	}
	return
}

func (ln *UnixListener) acceptOneshot() (c *UnixConn, err error) {

	// accept
	addr := &syscall.RawSockaddrAny{}
	addrLen := syscall.SizeofSockaddrAny

	cfd, acceptErr := ln.fd.Accept(addr, addrLen, ln.deadline)
	if acceptErr != nil {
		if aio.IsCanceled(acceptErr) {
			acceptErr = net.ErrClosed
		}
		err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: acceptErr}
		return
	}

	// unix conn
	c = &UnixConn{
		conn{
			fd:            cfd,
			readDeadline:  time.Time{},
			writeDeadline: time.Time{},
			useSendZC:     ln.useSendZC,
		},
		false,
	}
	return
}

func (ln *UnixListener) acceptMultishot() (c *UnixConn, err error) {
	ctx := ln.fd.Context()
	cfd, _, acceptErr := ln.acceptFuture.Await(ctx)
	if acceptErr != nil {
		if aio.IsCanceled(acceptErr) {
			acceptErr = net.ErrClosed
		}
		err = &net.OpError{Op: "accept", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: acceptErr}
		return
	}

	// unix conn
	c = &UnixConn{
		conn{
			fd:            cfd,
			readDeadline:  time.Time{},
			writeDeadline: time.Time{},
			useSendZC:     ln.useSendZC,
		},
		false,
	}
	return
}

func (ln *UnixListener) prepareMultishotAccepting() (err error) {
	addr := &syscall.RawSockaddrAny{}
	addrLen := syscall.SizeofSockaddrAny
	backlog := sys.MaxListenerBacklog()
	if backlog < 1024 {
		backlog = 1024
	}
	future := ln.fd.AcceptMultishotAsync(addr, addrLen, backlog)
	ln.acceptFuture = &future
	return
}

// Close stops listening on the Unix address. Already accepted
// connections are not closed.
func (ln *UnixListener) Close() error {
	if !ln.ok() {
		return syscall.EINVAL
	}

	if err := ln.fd.Close(); err != nil {
		return &net.OpError{Op: "close", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
	}
	return nil
}

// Addr returns the listener's network address, a [*TCPAddr].
// The Addr returned is shared by all invocations of Addr, so
// do not modify it.
func (ln *UnixListener) Addr() net.Addr {
	if !ln.ok() {
		return nil
	}
	return ln.fd.LocalAddr()
}

// SetDeadline sets the deadline associated with the listener.
// A zero time value disables the deadline.
// Only valid when not multishot accept mode.
func (ln *UnixListener) SetDeadline(t time.Time) error {
	if !ln.ok() {
		return syscall.EINVAL
	}
	ln.deadline = t
	return nil
}

// SetUnlinkOnClose sets whether the underlying socket file should be removed
// from the file system when the listener is closed.
//
// The default behavior is to unlink the socket file only when package net created it.
// That is, when the listener and the underlying socket file were created by a call to
// Listen or ListenUnix, then by default closing the listener will remove the socket file.
// but if the listener was created by a call to FileListener to use an already existing
// socket file, then by default closing the listener will not remove the socket file.
func (ln *UnixListener) SetUnlinkOnClose(unlink bool) {
	ln.unlink = unlink
}

// File returns a copy of the underlying [os.File].
// It is the caller's responsibility to close f when finished.
// Closing l does not affect f, and closing f does not affect l.
//
// The returned os.File's file descriptor is different from the
// connection's. Attempting to change properties of the original
// using this duplicate may or may not have the desired effect.
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

// SyscallConn returns a raw network connection.
// This implements the [syscall.Conn] interface.
//
// The returned RawConn only supports calling Control. Read and
// Write return an error.
func (ln *UnixListener) SyscallConn() (syscall.RawConn, error) {
	if !ln.ok() {
		return nil, syscall.EINVAL
	}
	return sys.NewRawConn(ln.fd.RegularSocket()), nil
}

// UnixConn is an implementation of the [net.Conn] interface for connections
// to Unix domain sockets.
type UnixConn struct {
	conn
	useSendMSGZC bool
}

// UseSendMSGZC try to enable sendmsg_zc.
func (c *UnixConn) UseSendMSGZC(use bool) bool {
	if !c.ok() {
		return false
	}
	if use {
		use = aio.CheckSendMsdZCEnable()
	}
	c.useSendMSGZC = use
	return use
}

// ReadFrom implements the [net.PacketConn] ReadFrom method.
func (c *UnixConn) ReadFrom(b []byte) (int, net.Addr, error) {
	return c.ReadFromUnix(b)
}

// ReadFromUnix acts like [UnixConn.ReadFrom] but returns a [net.UnixAddr].
func (c *UnixConn) ReadFromUnix(b []byte) (n int, addr *net.UnixAddr, err error) {
	if !c.ok() {
		return 0, nil, syscall.EINVAL
	}
	if len(b) == 0 {
		return 0, nil, &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}

	rsa := &syscall.RawSockaddrAny{}
	rsaLen := syscall.SizeofSockaddrAny

	deadline := c.deadline(c.fd.Context(), c.readDeadline)

	n, err = c.fd.ReceiveFrom(b, rsa, rsaLen, deadline)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			err = net.ErrClosed
		}
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

// ReadMsgUnix reads a message from c, copying the payload into b and
// the associated out-of-band data into oob. It returns the number of
// bytes copied into b, the number of bytes copied into oob, the flags
// that were set on the message and the source address of the message.
//
// Note that if len(b) == 0 and len(oob) > 0, this function will still
// read (and discard) 1 byte from the connection.
func (c *UnixConn) ReadMsgUnix(b []byte, oob []byte) (n, oobn, flags int, addr *net.UnixAddr, err error) {
	if !c.ok() {
		return 0, 0, 0, nil, syscall.EINVAL
	}
	bLen := len(b)
	oobLen := len(oob)
	if bLen == 0 && oobLen == 0 {
		return 0, 0, 0, nil, &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}

	rsa := &syscall.RawSockaddrAny{}
	rsaLen := syscall.SizeofSockaddrAny

	deadline := c.deadline(c.fd.Context(), c.readDeadline)

	n, oobn, flags, err = c.fd.ReceiveMsg(b, oob, rsa, rsaLen, unix.MSG_CMSG_CLOEXEC, deadline)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			err = net.ErrClosed
		}
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

// WriteTo implements the [net.PacketConn] WriteTo method.
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

// WriteToUnix acts like [UnixConn.WriteTo] but takes a [net.UnixAddr].
func (c *UnixConn) WriteToUnix(b []byte, addr *net.UnixAddr) (int, error) {
	return c.WriteTo(b, addr)
}

func (c *UnixConn) writeTo(b []byte, addr syscall.Sockaddr) (n int, err error) {
	rsa, rsaLen, rsaErr := sys.SockaddrToRawSockaddrAny(addr)
	if rsaErr != nil {
		return 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: rsaErr}
	}

	deadline := c.deadline(c.fd.Context(), c.writeDeadline)
	if c.useSendMSGZC {
		n, err = c.fd.SendToZC(b, rsa, int(rsaLen), deadline)
	} else {
		n, err = c.fd.SendTo(b, rsa, int(rsaLen), deadline)
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			err = net.ErrClosed
		}
		err = &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
		return
	}
	return
}

// WriteMsgUnix writes a message to addr via c, copying the payload
// from b and the associated out-of-band data from oob. It returns the
// number of payload and out-of-band bytes written.
//
// Note that if len(b) == 0 and len(oob) > 0, this function will still
// write 1 byte to the connection.
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

	deadline := c.deadline(c.fd.Context(), c.writeDeadline)
	if c.useSendMSGZC {
		n, oobn, err = c.fd.SendMsgZC(b, oob, rsa, int(rsaLen), deadline)
	} else {
		n, oobn, err = c.fd.SendMsg(b, oob, rsa, int(rsaLen), deadline)
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			err = net.ErrClosed
		}
		err = &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
		return
	}
	return
}
