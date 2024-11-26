//go:build windows

package aio

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"syscall"
	"unsafe"
)

func Accept(fd NetFd, cb OperationCallback) {
	// conn
	sock, sockErr := newSocket(fd.Family(), fd.SocketType(), fd.Protocol())
	if sockErr != nil {
		cb(0, Userdata{}, errors.Join(errors.New("aio: accept failed"), sockErr))
		return
	}
	// op
	op := fd.ReadOperator()
	op.userdata.Fd = &netFd{
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
	conn, _ := userdata.Fd.(*netFd)
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
