//go:build windows

package sockets

import (
	"errors"
	"golang.org/x/sys/windows"
	"io"
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

func (ln *tcpListener) Accept(handler AcceptHandler) {
	// socket
	connFd, createSocketErr := windows.WSASocket(windows.AF_INET, windows.SOCK_STREAM, 0, nil, 0, windows.WSA_FLAG_OVERLAPPED|windows.WSA_FLAG_NO_HANDLE_INHERIT)
	if createSocketErr != nil {
		handler(nil, wrapSyscallError("WSASocket", createSocketErr))
		return
	}
	// op
	op := &operation{
		Overlapped: windows.Overlapped{},
		mode:       accept,
		conn: connection{
			cphandle:   0,
			fd:         connFd,
			localAddr:  nil,
			remoteAddr: nil,
			net:        ln.net,
			sop:        nil,
			rop:        nil,
			wop:        nil,
		},
		buf:                        windows.WSABuf{},
		msg:                        windows.WSAMsg{},
		sa:                         nil,
		rsa:                        nil,
		rsan:                       0,
		iocp:                       0,
		handle:                     ln.fd,
		flags:                      0,
		bufs:                       nil,
		acceptHandler:              handler,
		readHandler:                nil,
		writeHandler:               nil,
		readFromHandler:            nil,
		readFromUDPHandler:         nil,
		readFromUDPAddrPortHandler: nil,
		readMsgUDPHandler:          nil,
		readMsgUDPAddrPortHandler:  nil,
		writeMsgHandler:            nil,
		readFromUnixHandler:        nil,
		readMsgUnixHandler:         nil,
		unixAcceptHandler:          nil,
	}
	op.conn.sop = op
	// sa
	var rawsa [2]windows.RawSockaddrAny
	lsan := uint32(unsafe.Sizeof(rawsa[1]))
	rsa := &rawsa[0]
	rsan := uint32(unsafe.Sizeof(rawsa[0]))
	// qty
	qty := uint32(0)
	// accept
	acceptErr := windows.AcceptEx(
		ln.fd, connFd,
		(*byte)(unsafe.Pointer(rsa)), 0,
		lsan+16, rsan+16,
		&qty, &op.Overlapped,
	)
	if acceptErr != nil && !errors.Is(windows.ERROR_IO_PENDING, acceptErr) {
		handler(nil, wrapSyscallError("AcceptEx", acceptErr))
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

func (conn *tcpConnection) Read(p []byte, handler ReadHandler) (err error) {
	// todo set op into conn
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
