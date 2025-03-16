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

func ListenUnix(network string, addr *net.UnixAddr) (*UnixListener, error) {
	config := ListenConfig{}
	ctx := context.Background()
	return config.ListenUnix(ctx, network, addr)
}

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
				_ = aio.Release(vortex)
				return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: regErr}
			}
		}
	}
	// send zc
	useSendZC := false
	useSendMSGZC := false
	if lc.UseSendZC {
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
			pinned:        true,
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
}

func (ln *UnixListener) Accept() (net.Conn, error) {
	return ln.AcceptUnix()
}

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
			pinned:        false,
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
			pinned:        false,
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

func (ln *UnixListener) Close() error {
	if !ln.ok() {
		return syscall.EINVAL
	}

	defer ln.cancel()

	ctx := ln.ctx
	vortex := ln.vortex

	if ln.useMultishotAccept {
		op := ln.acceptFuture.Operation()
		vortex.Cancel(ctx, op)
	}

	ln.unlinkOnce.Do(func() {
		if ln.path[0] != '@' && ln.unlink {
			_ = syscall.Unlink(ln.path)
		}
	})

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

func (ln *UnixListener) FixedFdInstalled() bool {
	return ln.fdFixed
}

func (ln *UnixListener) Addr() net.Addr {
	if !ln.ok() {
		return nil
	}
	return ln.fd.LocalAddr()
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
	useSendMSGZC bool
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
