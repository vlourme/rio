//go:build windows

package aio

import (
	"errors"
	"golang.org/x/sys/windows"
	"net"
	"os"
	"syscall"
	"unsafe"
)

func newListener(network string, family int, addr net.Addr, proto int) (fd NetFd, err error) {
	// create listener sock
	sock, sockErr := newSocket(family, windows.SOCK_STREAM, proto)
	if sockErr != nil {
		err = sockErr
		return
	}
	// set default opts
	setOptErr := setDefaultListenerSocketOpts(sock)
	if setOptErr != nil {
		err = setOptErr
		return
	}
	handle := syscall.Handle(sock)
	// bind
	sockaddr := AddrToSockaddr(addr)
	bindErr := syscall.Bind(handle, sockaddr)
	if bindErr != nil {
		err = os.NewSyscallError("bind", bindErr)
		_ = syscall.Closesocket(handle)
		return
	}
	// listen
	listenErr := syscall.Listen(handle, windows.SOMAXCONN)
	if listenErr != nil {
		err = os.NewSyscallError("listen", listenErr)
		_ = syscall.Closesocket(handle)
		return
	}
	// lsa
	lsa, getLSAErr := syscall.Getsockname(handle)
	if getLSAErr != nil {
		err = os.NewSyscallError("getsockname", getLSAErr)
		_ = syscall.Closesocket(handle)
		return
	}
	addr = SockaddrToAddr(network, lsa)

	// create listener iocp
	_, createListenIOCPErr := createSubIoCompletionPort(windows.Handle(sock))
	if createListenIOCPErr != nil {
		err = os.NewSyscallError("CreateIoCompletionPort", createListenIOCPErr)
		_ = syscall.Closesocket(handle)
		return
	}

	// fd
	sfd := &SocketFd{
		handle:     sock,
		network:    network,
		family:     family,
		socketType: syscall.SOCK_STREAM,
		protocol:   proto,
		localAddr:  addr,
		remoteAddr: nil,
		rop:        Operator{},
		wop:        Operator{},
	}
	sfd.rop.fd = sfd
	sfd.wop.fd = sfd

	fd = sfd
	return
}

func Accept(fd NetFd, cb OperationCallback) {
	// conn
	sock, sockErr := newSocket(fd.Family(), fd.SocketType(), fd.Protocol())
	if sockErr != nil {
		cb(0, Userdata{}, errors.Join(errors.New("aio: accept failed"), sockErr))
		return
	}
	// op
	op := fd.ReadOperator()
	op.userdata.Fd = &SocketFd{
		handle:     sock,
		network:    fd.Network(),
		family:     fd.Family(),
		socketType: fd.SocketType(),
		protocol:   fd.Protocol(),
		localAddr:  nil,
		remoteAddr: nil,
	}
	// callback
	op.callback = cb
	// completion
	op.completion = completeAccept

	// overlapped
	overlapped := &op.overlapped

	// sa
	var rawsa [2]windows.RawSockaddrAny
	lsan := uint32(unsafe.Sizeof(rawsa[1]))
	rsa := &rawsa[0]
	rsan := uint32(unsafe.Sizeof(rawsa[0]))

	// timeout
	if op.timeout > 0 {
		timer := getOperatorTimer()
		op.timer = timer
		timer.Start(op.timeout, &operatorCanceler{
			handle:     syscall.Handle(sock),
			overlapped: overlapped,
		})
	}

	// accept
	acceptErr := syscall.AcceptEx(
		syscall.Handle(fd.Fd()), syscall.Handle(sock),
		(*byte)(unsafe.Pointer(rsa)), 0,
		lsan+16, rsan+16,
		&op.userdata.QTY, overlapped,
	)
	if acceptErr != nil && !errors.Is(windows.ERROR_IO_PENDING, acceptErr) {
		_ = windows.Closesocket(windows.Handle(sock))
		cb(0, op.userdata, errors.Join(errors.New("aio: accept failed"), acceptErr))

		op.callback = nil
		op.completion = nil
		if op.timer != nil {
			timer := op.timer
			timer.Done()
			putOperatorTimer(timer)
			op.timer = nil
		}
	}

	return
}

func completeAccept(result int, op *Operator, err error) {
	userdata := op.userdata
	// conn
	conn, _ := userdata.Fd.(*SocketFd)
	connFd := syscall.Handle(conn.handle)
	if err != nil {
		_ = syscall.Closesocket(connFd)
		op.callback(result, userdata, os.NewSyscallError("iocp.AcceptEx", err))
		return
	}
	// ln
	ln, _ := op.fd.(NetFd)
	lnFd := syscall.Handle(ln.Fd())

	// set SO_UPDATE_ACCEPT_CONTEXT
	setAcceptSocketOptErr := syscall.Setsockopt(
		connFd,
		syscall.SOL_SOCKET, syscall.SO_UPDATE_ACCEPT_CONTEXT,
		(*byte)(unsafe.Pointer(&lnFd)),
		int32(unsafe.Sizeof(lnFd)),
	)
	if setAcceptSocketOptErr != nil {
		_ = syscall.Closesocket(connFd)
		op.callback(result, userdata, os.NewSyscallError("setsockopt", setAcceptSocketOptErr))
		return
	}

	// get local addr
	lsa, lsaErr := syscall.Getsockname(connFd)
	if lsaErr != nil {
		_ = syscall.Closesocket(connFd)
		op.callback(result, userdata, os.NewSyscallError("getsockname", lsaErr))
		return
	}
	la := SockaddrToAddr(ln.Network(), lsa)
	conn.localAddr = la

	// get remote addr
	rsa, rsaErr := syscall.Getpeername(connFd)
	if rsaErr != nil {
		_ = syscall.Closesocket(connFd)
		op.callback(result, userdata, os.NewSyscallError("getsockname", rsaErr))
		return
	}
	ra := SockaddrToAddr(ln.Network(), rsa)
	conn.remoteAddr = ra

	// create iocp
	_, iocpErr := createSubIoCompletionPort(windows.Handle(connFd))
	if iocpErr != nil {
		_ = syscall.Closesocket(connFd)
		op.callback(result, userdata, iocpErr)
		return
	}

	// callback
	op.callback(conn.handle, userdata, err)
	return
}
