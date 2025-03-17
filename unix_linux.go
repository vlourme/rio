//go:build linux

package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"github.com/brickingsoft/rio/pkg/sys"
	"golang.org/x/sys/unix"
	"net"
	"os"
	"reflect"
	"sync"
	"sync/atomic"
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
	switch network {
	case "unix", "unixpacket":
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
	var control ctrlCtxFn = nil
	if lc.Control != nil {
		control = func(ctx context.Context, network string, address string, raw syscall.RawConn) error {
			return lc.Control(network, address, raw)
		}
	}
	fd, fdErr := newUnixListener(ctx, network, addr, control)
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
	ln := &UnixListener{
		ctx:                cc,
		cancel:             cancel,
		fd:                 fd,
		autoFixedFdInstall: autoFixedFdInstall,
		fdFixed:            fileIndex != -1,
		fileIndex:          fileIndex,
		sqeFlags:           sqeFlags,
		path:               fd.LocalAddr().String(),
		unlink:             true,
		unlinkOnce:         sync.Once{},
		vortex:             vortex,
		acceptFuture:       nil,
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
	var control ctrlCtxFn = nil
	if lc.Control != nil {
		control = func(ctx context.Context, network string, address string, raw syscall.RawConn) error {
			return lc.Control(network, address, raw)
		}
	}
	fd, fdErr := newUnixListener(ctx, network, addr, control)
	if fdErr != nil {
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: fdErr}
	}
	// install fixed fd
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
		}
	}
	// send zc
	useSendZC := false
	useSendMSGZC := false
	if lc.SendZC {
		useSendZC = aio.CheckSendZCEnable()
		useSendMSGZC = aio.CheckSendMsdZCEnable()
	}
	// conn
	cc, cancel := context.WithCancel(ctx)
	c := &UnixConn{
		conn{
			ctx:           cc,
			cancel:        cancel,
			fd:            fd,
			fdFixed:       fileIndex != -1,
			fileIndex:     fileIndex,
			sqeFlags:      sqeFlags,
			vortex:        vortex,
			readDeadline:  time.Time{},
			writeDeadline: time.Time{},
			readBuffer:    atomic.Int64{},
			writeBuffer:   atomic.Int64{},
			useSendZC:     useSendZC,
		},
		useSendMSGZC,
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

// UnixListener is a Unix domain socket listener. Clients should
// typically use variables of type [net.Listener] instead of assuming Unix
// domain sockets.
type UnixListener struct {
	ctx                context.Context
	cancel             context.CancelFunc
	fd                 *sys.Fd
	fdFixed            bool
	fileIndex          int
	sqeFlags           uint8
	path               string
	unlink             bool
	unlinkOnce         sync.Once
	vortex             *aio.Vortex
	acceptFuture       *aio.Future
	useSendZC          bool
	useMultishotAccept bool
	autoFixedFdInstall bool
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
	// unix conn
	cc, cancel := context.WithCancel(ctx)
	c = &UnixConn{
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
		false,
	}
	return
}

func (ln *UnixListener) acceptMultishot() (c *UnixConn, err error) {
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
	// unix conn
	cc, cancel := context.WithCancel(ctx)
	c = &UnixConn{
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
		false,
	}
	return
}

func (ln *UnixListener) prepareMultishotAccepting() (err error) {
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

// Close stops listening on the Unix address. Already accepted
// connections are not closed.
func (ln *UnixListener) Close() error {
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

	ln.unlinkOnce.Do(func() {
		if ln.path[0] != '@' && ln.unlink {
			_ = syscall.Unlink(ln.path)
		}
	})

	if err != nil {
		_ = syscall.Close(fd)
		return &net.OpError{Op: "close", Net: ln.fd.Net(), Source: nil, Addr: ln.fd.LocalAddr(), Err: err}
	}
	return nil
}

// InstallFixedFd implements the InstallFixedFd method in the [FixedFd] interface.
func (ln *UnixListener) InstallFixedFd() error {
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
func (ln *UnixListener) FixedFdInstalled() bool {
	return ln.fdFixed
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
	return newRawConn(ln.fd), nil
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

	ctx := c.ctx
	fd := 0
	if c.fdFixed {
		fd = c.fileIndex
	} else {
		fd = c.fd.Socket()
	}
	vortex := c.vortex

	rsa := &syscall.RawSockaddrAny{}
	rsaLen := syscall.SizeofSockaddrAny

	deadline := c.deadline(ctx, c.readDeadline)

	n, err = vortex.ReceiveFrom(ctx, fd, b, rsa, rsaLen, deadline, c.sqeFlags)
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

	ctx := c.ctx
	fd := 0
	if c.fdFixed {
		fd = c.fileIndex
	} else {
		fd = c.fd.Socket()
	}
	vortex := c.vortex

	rsa := &syscall.RawSockaddrAny{}
	rsaLen := syscall.SizeofSockaddrAny

	deadline := c.deadline(ctx, c.readDeadline)

	n, oobn, flags, err = vortex.ReceiveMsg(ctx, fd, b, oob, rsa, rsaLen, unix.MSG_CMSG_CLOEXEC, deadline, c.sqeFlags)
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

	ctx := c.ctx
	fd := 0
	if c.fdFixed {
		fd = c.fileIndex
	} else {
		fd = c.fd.Socket()
	}
	vortex := c.vortex

	deadline := c.deadline(ctx, c.writeDeadline)
	if c.useSendMSGZC {
		n, err = vortex.SendToZC(ctx, fd, b, rsa, int(rsaLen), deadline, c.sqeFlags)
	} else {
		n, err = vortex.SendTo(ctx, fd, b, rsa, int(rsaLen), deadline, c.sqeFlags)
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

	ctx := c.ctx
	fd := 0
	if c.fdFixed {
		fd = c.fileIndex
	} else {
		fd = c.fd.Socket()
	}
	vortex := c.vortex

	deadline := c.deadline(ctx, c.writeDeadline)
	if c.useSendMSGZC {
		n, oobn, err = vortex.SendMsgZC(ctx, fd, b, oob, rsa, int(rsaLen), deadline, c.sqeFlags)
	} else {
		n, oobn, err = vortex.SendMsg(ctx, fd, b, oob, rsa, int(rsaLen), deadline, c.sqeFlags)
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
