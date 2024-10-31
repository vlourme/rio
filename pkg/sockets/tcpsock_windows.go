//go:build windows

package sockets

import (
	"errors"
	"golang.org/x/sys/windows"
	"io"
	"net"
	"os"
	"runtime"
	"unsafe"
)

func newTCPListener(network string, addr *net.TCPAddr, proto int, pollers int) (ln *tcpListener, err error) {
	// startup wsa
	_, startupErr := wsaStartup()
	if startupErr != nil {
		err = os.NewSyscallError("WSAStartup", startupErr)
		return
	}
	// create root iocp
	iocp, createIOCPErr := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, 0)
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
		_ = windows.CloseHandle(iocp)
		wsaCleanup()
		return
	}
	// set default opts
	setDefaultSockOptsErr := setDefaultSockopts(fd, family, windows.SOCK_STREAM, ipv6only)
	if setDefaultSockOptsErr != nil {
		err = setDefaultSockOptsErr
		_ = windows.Closesocket(fd)
		_ = windows.CloseHandle(iocp)
		wsaCleanup()
		return
	}
	// bind
	sockaddr := addrToSockaddr(family, addr)
	bindErr := windows.Bind(fd, sockaddr)
	if bindErr != nil {
		err = os.NewSyscallError("Bind", bindErr)
		_ = windows.Closesocket(fd)
		_ = windows.CloseHandle(iocp)
		wsaCleanup()
		return
	}

	// create listener iocp
	listenerIOCP, createListenIOCPErr := windows.CreateIoCompletionPort(fd, iocp, 0, 0)
	if createListenIOCPErr != nil {
		err = os.NewSyscallError("CreateIoCompletionPort", createListenIOCPErr)
		_ = windows.Closesocket(fd)
		_ = windows.CloseHandle(iocp)
		wsaCleanup()
		return
	}

	// create listener
	if pollers < 1 {
		pollers = runtime.NumCPU() * 2
	}
	ln = &tcpListener{
		rootIOCP: iocp,
		iocp:     listenerIOCP,
		fd:       fd,
		addr:     addr,
		family:   family,
		net:      network,
		pollers:  pollers,
	}
	// polling
	ln.polling()
	return
}

type tcpListener struct {
	rootIOCP windows.Handle
	iocp     windows.Handle
	fd       windows.Handle
	addr     *net.TCPAddr
	family   int
	net      string
	pollers  int
}

func (ln *tcpListener) Addr() (addr net.Addr) {
	addr = ln.addr
	return
}

func (ln *tcpListener) Accept(handler AcceptHandler) {
	//TODO implement me
	panic("implement me")
}

func (ln *tcpListener) Close() (err error) {
	defer func() {
		_ = windows.WSACleanup()
	}()
	for i := 0; i < ln.pollers; i++ {
		_ = windows.PostQueuedCompletionStatus(ln.fd, 0, 0, nil)
	}
	// break polling
	// todo test post status times by 1 (1 -> 1)
	// close socket
	closeSockErr := windows.Closesocket(ln.fd)
	if closeSockErr != nil {
		err = &net.OpError{Op: "close", Net: ln.net, Source: nil, Addr: ln.addr, Err: closeSockErr}
		return
	}

	closeIocpErr := windows.Close(ln.iocp)
	if closeIocpErr != nil {
		err = &net.OpError{Op: "close", Net: ln.net, Source: nil, Addr: ln.addr, Err: closeIocpErr}
		return
	}

	//TODO implement me
	panic("implement me")
}

func (ln *tcpListener) polling() {
	/*
		客户端主动关闭：qty是0 io.EOF
		客户端异常关闭：windows.ERROR_NETNAME_DELETED
		服务器主动关闭：windows.ERROR_CONNECTION_ABORTED
		超时: windows.WAIT_TIMEOUT
	*/
	for i := 0; i < ln.pollers; i++ {
		go func(ln *tcpListener) {
			var qty uint32
			var key uintptr
			var overlapped *windows.Overlapped
			for {
				getQueuedCompletionStatusErr := windows.GetQueuedCompletionStatus(ln.iocp, &qty, &key, &overlapped, windows.INFINITE)
				op := (*operation)(unsafe.Pointer(overlapped))
				if getQueuedCompletionStatusErr != nil {
					op.failed(wrapSyscallError("GetQueuedCompletionStatus", getQueuedCompletionStatusErr))
					continue
				}
				if qty == 0 { // normal closed by client
					if op.mode == exit {
						break
					}
					op.failed(io.EOF)
					continue
				}
				// todo
				switch op.mode {
				case accept:
					break
				case read:
					break
				case write:
					break
				case disconnect:
					break
				default:
					// not supported
					op.failed(wrapSyscallError("GetQueuedCompletionStatus", errors.New("invalid operation")))
					break
				}
			}
		}(ln)
	}
}

func (ln *tcpListener) handleAccept(op *operation) {
	// todo
	return
}

type tcpConnection struct {
	iocp       windows.Handle // conn iocp
	fd         windows.Handle
	localAddr  net.Addr
	remoteAddr net.Addr
	family     int
	sotype     int
	net        string
}

func (conn *tcpConnection) Close() {

	//
	_ = windows.Shutdown(conn.fd, 2)
	_ = windows.Closesocket(conn.fd)
	_ = windows.CloseHandle(conn.iocp)
}
