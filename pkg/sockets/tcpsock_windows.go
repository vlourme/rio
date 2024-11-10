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

func connectTCP(network string, family int, addr *net.TCPAddr, ipv6only bool, proto int, handler TCPDialHandler) {
	// conn
	conn, connErr := newConnection(network, family, windows.SOCK_STREAM, proto, ipv6only)
	if connErr != nil {
		handler(nil, connErr)
		return
	}
	conn.rop.mode = tcpConnect
	conn.rop.tcpConnectHandler = handler
	// lsa
	var lsa windows.Sockaddr
	if ipv6only {
		lsa = &windows.SockaddrInet6{}
	} else {
		lsa = &windows.SockaddrInet4{}
	}
	// bind
	bindErr := windows.Bind(conn.fd, lsa)
	if bindErr != nil {
		handler(nil, os.NewSyscallError("bind", bindErr))
		return
	}
	// CreateIoCompletionPort
	cphandle, createErr := windows.CreateIoCompletionPort(conn.fd, iocp, key, 0)
	if createErr != nil {
		handler(nil, wrapSyscallError("createIoCompletionPort", createErr))
		conn.rop.tcpConnectHandler = nil
		return
	}
	conn.cphandle = cphandle
	// ra
	rsa := addrToSockaddr(family, addr)
	// overlapped
	overlapped := &conn.rop.overlapped
	// connect
	connectErr := windows.ConnectEx(conn.fd, rsa, nil, 0, nil, overlapped)
	if connectErr != nil && !errors.Is(connectErr, windows.ERROR_IO_PENDING) {
		handler(nil, wrapSyscallError("ConnectEx", connectErr))
		conn.rop.tcpConnectHandler = nil
		return
	}
}

func newTCPListener(network string, family int, addr *net.TCPAddr, ipv6only bool, proto int) (ln *tcpListener, err error) {
	// create listener fd
	fd, fdErr := windows.WSASocket(int32(family), windows.SOCK_STREAM, int32(proto), nil, 0, windows.WSA_FLAG_OVERLAPPED|windows.WSA_FLAG_NO_HANDLE_INHERIT)
	if fdErr != nil {
		err = os.NewSyscallError("WSASocket", fdErr)
		return
	}
	// set default opts
	setDefaultSockOptsErr := setDefaultSockopts(fd, family, windows.SOCK_STREAM, ipv6only)
	if setDefaultSockOptsErr != nil {
		err = setDefaultSockOptsErr
		_ = windows.Closesocket(fd)
		return
	}
	// bind
	sockaddr := addrToSockaddr(family, addr)
	bindErr := windows.Bind(fd, sockaddr)
	if bindErr != nil {
		err = os.NewSyscallError("Bind", bindErr)
		_ = windows.Closesocket(fd)
		return
	}
	// listen
	listenErr := windows.Listen(fd, windows.SOMAXCONN)
	if listenErr != nil {
		err = os.NewSyscallError("Listen", listenErr)
		_ = windows.Closesocket(fd)
		return
	}
	// lsa
	lsa, getLSAErr := windows.Getsockname(fd)
	if getLSAErr != nil {
		err = os.NewSyscallError("Getsockname", getLSAErr)
		_ = windows.Closesocket(fd)
		return
	}
	addr = sockaddrToTCPAddr(lsa)

	// create listener iocp
	cphandler, createListenIOCPErr := createSubIoCompletionPort(fd)
	if createListenIOCPErr != nil {
		err = os.NewSyscallError("CreateIoCompletionPort", createListenIOCPErr)
		_ = windows.Closesocket(fd)
		return
	}

	// create listener
	ln = &tcpListener{
		cphandle: cphandler,
		fd:       fd,
		addr:     addr,
		family:   family,
		net:      network,
		ipv6only: ipv6only,
	}
	return
}

type tcpListener struct {
	cphandle windows.Handle
	fd       windows.Handle
	addr     *net.TCPAddr
	family   int
	net      string
	ipv6only bool
}

func (ln *tcpListener) Addr() (addr net.Addr) {
	addr = ln.addr
	return
}

func (ln *tcpListener) Accept(handler TCPAcceptHandler) {
	// conn
	conn, connErr := newConnection(ln.net, windows.AF_INET, windows.SOCK_STREAM, 0, ln.ipv6only)
	if connErr != nil {
		handler(nil, connErr)
		return
	}
	// op
	conn.rop.mode = tcpAccept
	conn.rop.handle = ln.fd
	conn.rop.iocp = ln.cphandle
	conn.rop.tcpAcceptHandler = handler
	// sa
	var rawsa [2]windows.RawSockaddrAny
	lsan := uint32(unsafe.Sizeof(rawsa[1]))
	rsa := &rawsa[0]
	rsan := uint32(unsafe.Sizeof(rawsa[0]))
	// overlapped
	overlapped := &conn.rop.overlapped
	// tcpAccept
	acceptErr := windows.AcceptEx(
		ln.fd, conn.fd,
		(*byte)(unsafe.Pointer(rsa)), 0,
		lsan+16, rsan+16,
		&conn.rop.qty, overlapped,
	)
	if acceptErr != nil && !errors.Is(windows.ERROR_IO_PENDING, acceptErr) {
		handler(nil, wrapSyscallError("AcceptEx", acceptErr))
		conn.rop.tcpAcceptHandler = nil
	}
}

func (ln *tcpListener) Close() (err error) {
	// close socket
	closeSockErr := windows.Closesocket(ln.fd)
	if closeSockErr != nil {
		err = wrapSyscallError("closesocket", closeSockErr)
	}
	return
}

type tcpConnection struct {
	connection
}

func (conn *tcpConnection) Read(p []byte, handler ReadHandler) {
	pLen := len(p)
	if pLen == 0 {
		handler(0, errors.New("rio: empty packet"))
		return
	} else if pLen > maxRW {
		p = p[:maxRW]
		pLen = maxRW
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
	} else if pLen > maxRW {
		p = p[:maxRW]
		pLen = maxRW
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
