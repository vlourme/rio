//go:build windows

package sockets

import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"net"
	"os"
	"time"
	"unsafe"
)

func newTCPListener(network string, family int, addr *net.TCPAddr, ipv6only bool, proto int, pollers int) (ln *tcpListener, err error) {
	// startup wsa
	_, startupErr := wsaStartup()
	if startupErr != nil {
		err = os.NewSyscallError("WSAStartup", startupErr)
		return
	}
	// create root iocp
	cphandle, createIOCPErr := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, 0)
	if createIOCPErr != nil {
		err = os.NewSyscallError("CreateIoCompletionPort", createIOCPErr)
		wsaCleanup()
		return
	}
	// create listener fd
	fd, fdErr := windows.WSASocket(int32(family), windows.SOCK_STREAM, int32(proto), nil, 0, windows.WSA_FLAG_OVERLAPPED|windows.WSA_FLAG_NO_HANDLE_INHERIT)
	if fdErr != nil {
		err = os.NewSyscallError("WSASocket", fdErr)
		_ = windows.CloseHandle(cphandle)
		wsaCleanup()
		return
	}
	// set default opts
	setDefaultSockOptsErr := setDefaultSockopts(fd, family, windows.SOCK_STREAM, ipv6only)
	if setDefaultSockOptsErr != nil {
		err = setDefaultSockOptsErr
		_ = windows.Closesocket(fd)
		_ = windows.CloseHandle(cphandle)
		wsaCleanup()
		return
	}
	// bind
	sockaddr := addrToSockaddr(family, addr)
	bindErr := windows.Bind(fd, sockaddr)
	if bindErr != nil {
		err = os.NewSyscallError("Bind", bindErr)
		_ = windows.Closesocket(fd)
		_ = windows.CloseHandle(cphandle)
		wsaCleanup()
		return
	}
	// listen
	listenErr := windows.Listen(fd, windows.SOMAXCONN)
	if listenErr != nil {
		err = os.NewSyscallError("Listen", listenErr)
		_ = windows.Closesocket(fd)
		_ = windows.CloseHandle(cphandle)
		wsaCleanup()
		return
	}
	// lsa
	lsa, getLSAErr := windows.Getsockname(fd)
	if getLSAErr != nil {
		err = os.NewSyscallError("Getsockname", getLSAErr)
		_ = windows.Closesocket(fd)
		_ = windows.CloseHandle(cphandle)
		wsaCleanup()
		return
	}
	addr = sockaddrToTCPAddr(lsa)

	// create listener iocp
	_, createListenIOCPErr := windows.CreateIoCompletionPort(fd, cphandle, 0, 0)
	if createListenIOCPErr != nil {
		err = os.NewSyscallError("CreateIoCompletionPort", createListenIOCPErr)
		_ = windows.Closesocket(fd)
		_ = windows.CloseHandle(cphandle)
		wsaCleanup()
		return
	}

	// create listener
	ln = &tcpListener{
		cphandle: cphandle,
		fd:       fd,
		addr:     addr,
		family:   family,
		net:      network,
		poller:   newPoller(pollers, cphandle),
	}
	// polling
	ln.polling()
	return
}

type tcpListener struct {
	cphandle windows.Handle
	fd       windows.Handle
	addr     *net.TCPAddr
	family   int
	net      string
	poller   *poller
}

func (ln *tcpListener) Addr() (addr net.Addr) {
	addr = ln.addr
	return
}

func (ln *tcpListener) Accept(handler TCPAcceptHandler) {
	// socket
	connFd, createSocketErr := windows.WSASocket(windows.AF_INET, windows.SOCK_STREAM, 0, nil, 0, windows.WSA_FLAG_OVERLAPPED|windows.WSA_FLAG_NO_HANDLE_INHERIT)
	if createSocketErr != nil {
		handler(nil, wrapSyscallError("WSASocket", createSocketErr))
		return
	}
	// conn
	conn := newConnection(ln.net, windows.SOCK_STREAM, connFd)
	// op
	conn.rop.mode = accept
	conn.rop.handle = ln.fd
	conn.rop.iocp = ln.cphandle
	conn.rop.acceptHandler = handler
	// sa
	var rawsa [2]windows.RawSockaddrAny
	lsan := uint32(unsafe.Sizeof(rawsa[1]))
	rsa := &rawsa[0]
	rsan := uint32(unsafe.Sizeof(rawsa[0]))
	// overlapped
	overlapped := &conn.rop.overlapped
	// accept
	acceptErr := windows.AcceptEx(
		ln.fd, connFd,
		(*byte)(unsafe.Pointer(rsa)), 0,
		lsan+16, rsan+16,
		&conn.rop.qty, overlapped,
	)
	if acceptErr != nil && !errors.Is(windows.ERROR_IO_PENDING, acceptErr) {
		handler(nil, wrapSyscallError("AcceptEx", acceptErr))
		conn.rop.acceptHandler = nil
	}
}

func (ln *tcpListener) Close() (err error) {
	defer func() {
		_ = windows.WSACleanup()
	}()
	// stop polling
	ln.poller.stop()
	// close socket
	closeSockErr := windows.Closesocket(ln.fd)
	if closeSockErr != nil {
		err = wrapSyscallError("closesocket", closeSockErr)
		return
	}
	// close iocp
	closeIocpErr := windows.CloseHandle(ln.cphandle)
	if closeIocpErr != nil {
		err = wrapSyscallError("CloseHandle", closeIocpErr)
		return
	}
	return
}

func (ln *tcpListener) polling() {
	ln.poller.start()
}

type tcpConnection struct {
	connection
}

func (conn *tcpConnection) Read(p []byte, handler ReadHandler) {
	pLen := len(p)
	if pLen == 0 {
		handler(0, errors.New("rio: empty packet"))
		return
	}
	conn.rop.mode = read
	conn.rop.buf.Buf = &p[0]
	conn.rop.buf.Len = uint32(pLen)
	conn.rop.readHandler = handler
	err := windows.WSARecv(conn.fd, &conn.rop.buf, 1, &conn.rop.qty, &conn.rop.flags, &conn.rop.overlapped, nil)
	if err != nil && !errors.Is(windows.ERROR_IO_PENDING, err) {
		if errors.Is(windows.ERROR_TIMEOUT, err) {
			err = context.DeadlineExceeded
		}
		handler(0, wrapSyscallError("WSARecv", err))
		conn.rop.readHandler = nil
	}
}

func (conn *tcpConnection) Write(p []byte, handler WriteHandler) {
	pLen := len(p)
	if pLen == 0 {
		handler(0, errors.New("rio: empty packet"))
		return
	}
	conn.wop.mode = write
	conn.wop.buf.Buf = &p[0]
	conn.wop.buf.Len = uint32(pLen)
	conn.wop.writeHandler = handler
	err := windows.WSASend(conn.fd, &conn.wop.buf, 1, &conn.wop.qty, conn.wop.flags, &conn.wop.overlapped, nil)
	if err != nil && !errors.Is(windows.ERROR_IO_PENDING, err) {
		if errors.Is(windows.ERROR_TIMEOUT, err) {
			err = context.DeadlineExceeded
		}
		handler(0, wrapSyscallError("WSASend", err))
		conn.wop.writeHandler = nil
	}
}

func (conn *tcpConnection) SetNoDelay(noDelay bool) (err error) {
	err = windows.SetsockoptInt(conn.fd, windows.IPPROTO_TCP, windows.TCP_NODELAY, boolint(noDelay))
	if err != nil {
		err = wrapSyscallError("setsockopt", err)
		return
	}
	return
}

func (conn *tcpConnection) SetLinger(sec int) (err error) {
	var l windows.Linger
	if sec >= 0 {
		l.Onoff = 1
		l.Linger = int32(sec)
	} else {
		l.Onoff = 0
		l.Linger = 0
	}
	err = windows.SetsockoptLinger(conn.fd, windows.SOL_SOCKET, windows.SO_LINGER, &l)
	if err != nil {
		err = wrapSyscallError("setsockopt", err)
		return
	}
	return
}

func (conn *tcpConnection) SetKeepAlive(keepalive bool) (err error) {
	err = windows.SetsockoptInt(conn.fd, windows.SOL_SOCKET, windows.SO_KEEPALIVE, boolint(keepalive))
	if err != nil {
		err = wrapSyscallError("setsockopt", err)
		return
	}
	return
}

const (
	defaultTCPKeepAliveIdle = 5 * time.Second
)

func (conn *tcpConnection) SetKeepAlivePeriod(period time.Duration) (err error) {
	if period == 0 {
		period = defaultTCPKeepAliveIdle
	} else if period < 0 {
		return nil
	}
	secs := int(roundDurationUp(period, time.Second))
	err = windows.SetsockoptInt(conn.fd, windows.IPPROTO_TCP, windows.TCP_KEEPIDLE, secs)
	if err != nil {
		err = wrapSyscallError("setsockopt", err)
		return
	}
	return
}
