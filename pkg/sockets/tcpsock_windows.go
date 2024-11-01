//go:build windows

package sockets

import (
	"golang.org/x/sys/windows"
	"io"
	"net"
	"os"
	"time"
)

func newTCPListener(network string, addr *net.TCPAddr, proto int, pollers int) (ln *tcpListener, err error) {
	// startup wsa
	_, startupErr := wsaStartup()
	if startupErr != nil {
		err = os.NewSyscallError("WSAStartup", startupErr)
		return
	}
	// create root iocp
	rcphandle, createIOCPErr := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, 0)
	if createIOCPErr != nil {
		err = os.NewSyscallError("CreateIoCompletionPort", createIOCPErr)
		wsaCleanup()
		return
	}
	// create listener fd
	family, ipv6only := getAddrFamily(network, addr)
	fd, fdErr := windows.WSASocket(int32(family), windows.SOCK_STREAM, int32(proto), nil, 0, windows.WSA_FLAG_OVERLAPPED|windows.WSA_FLAG_NO_HANDLE_INHERIT)
	if fdErr != nil {
		err = os.NewSyscallError("WSASocket", fdErr)
		_ = windows.CloseHandle(rcphandle)
		wsaCleanup()
		return
	}
	// set default opts
	setDefaultSockOptsErr := setDefaultSockopts(fd, family, windows.SOCK_STREAM, ipv6only)
	if setDefaultSockOptsErr != nil {
		err = setDefaultSockOptsErr
		_ = windows.Closesocket(fd)
		_ = windows.CloseHandle(rcphandle)
		wsaCleanup()
		return
	}
	// bind
	sockaddr := addrToSockaddr(family, addr)
	bindErr := windows.Bind(fd, sockaddr)
	if bindErr != nil {
		err = os.NewSyscallError("Bind", bindErr)
		_ = windows.Closesocket(fd)
		_ = windows.CloseHandle(rcphandle)
		wsaCleanup()
		return
	}
	// listen
	listenErr := windows.Listen(fd, windows.SOMAXCONN)
	if listenErr != nil {
		err = os.NewSyscallError("Listen", listenErr)
		_ = windows.Closesocket(fd)
		_ = windows.CloseHandle(rcphandle)
		wsaCleanup()
		return
	}
	// lsa
	lsa, getLSAErr := windows.Getsockname(fd)
	if getLSAErr != nil {
		err = os.NewSyscallError("Getsockname", getLSAErr)
		_ = windows.Closesocket(fd)
		_ = windows.CloseHandle(rcphandle)
		wsaCleanup()
		return
	}
	addr = sockaddrToTCPAddr(lsa)

	// create listener iocp
	cphandle, createListenIOCPErr := windows.CreateIoCompletionPort(fd, rcphandle, 0, 0)
	if createListenIOCPErr != nil {
		err = os.NewSyscallError("CreateIoCompletionPort", createListenIOCPErr)
		_ = windows.Closesocket(fd)
		_ = windows.CloseHandle(rcphandle)
		wsaCleanup()
		return
	}

	// create listener
	ln = &tcpListener{
		rcphandle: rcphandle,
		cphandle:  cphandle,
		fd:        fd,
		addr:      addr,
		family:    family,
		net:       network,
		poller:    newPoller(0, cphandle),
	}
	// polling
	ln.polling()
	return
}

type tcpListener struct {
	rcphandle windows.Handle
	cphandle  windows.Handle
	fd        windows.Handle
	addr      *net.TCPAddr
	family    int
	net       string
	poller    *poller
}

func (ln *tcpListener) Addr() (addr net.Addr) {
	addr = ln.addr
	return
}

func (ln *tcpListener) Accept(handler AcceptHandler) {
	// op.handler = ln.fd
	// op.iocp = ln.iocp
	//TODO implement me
	panic("implement me")
}

func (ln *tcpListener) Close() (err error) {
	defer func() {
		_ = windows.WSACleanup()
	}()

	ln.poller.stop()
	// close socket
	closeSockErr := windows.Closesocket(ln.fd)
	if closeSockErr != nil {
		err = &net.OpError{Op: "close", Net: ln.net, Source: nil, Addr: ln.addr, Err: closeSockErr}
		return
	}

	closeIocpErr := windows.Close(ln.cphandle)
	if closeIocpErr != nil {
		err = &net.OpError{Op: "close", Net: ln.net, Source: nil, Addr: ln.addr, Err: closeIocpErr}
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

func (conn *tcpConnection) Read(p []byte, handler ReadHandler) (err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *tcpConnection) Write(p []byte, handler WriteHandler) (err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *tcpConnection) ReadFrom(r io.Reader) (n int64, err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *tcpConnection) WriteTo(w io.Writer) (n int64, err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *tcpConnection) SetNoDelay(noDelay bool) (err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *tcpConnection) SetLinger(sec int) (err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *tcpConnection) SetKeepAlive(keepalive bool) (err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *tcpConnection) SetKeepAlivePeriod(d time.Duration) (err error) {
	//TODO implement me
	panic("implement me")
}
