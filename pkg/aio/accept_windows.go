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
	sock, sockErr := newSocket(fd.Family(), fd.SocketType(), fd.Protocol(), fd.IPv6Only())
	if sockErr != nil {
		cb(Userdata{}, sockErr)
		return
	}
	// op
	op := fd.ReadOperator()
	// set sock
	op.handle = sock
	// set callback
	op.callback = cb
	// set completion
	op.completion = completeAccept

	// timeout
	op.tryPrepareTimeout()
	
	// overlapped
	overlapped := &op.overlapped

	// sa
	var rawsa [2]syscall.RawSockaddrAny
	lsan := uint32(unsafe.Sizeof(rawsa[1]))
	rsa := &rawsa[0]
	rsan := uint32(unsafe.Sizeof(rawsa[0]))

	// accept
	acceptErr := syscall.AcceptEx(
		syscall.Handle(fd.Fd()), syscall.Handle(sock),
		(*byte)(unsafe.Pointer(rsa)), 0,
		lsan+16, rsan+16,
		&op.n, overlapped,
	)
	if acceptErr != nil && !errors.Is(syscall.ERROR_IO_PENDING, acceptErr) {
		_ = syscall.Closesocket(syscall.Handle(sock))
		cb(Userdata{}, os.NewSyscallError("acceptex", acceptErr))
		op.clean()
	}

	return
}

func completeAccept(_ int, op *Operator, err error) {
	// sock
	sock := syscall.Handle(op.handle)
	// handle error
	if err != nil {
		_ = syscall.Closesocket(sock)
		op.callback(Userdata{}, os.NewSyscallError("acceptex", err))
		return
	}
	// ln
	ln, _ := op.fd.(NetFd)
	lnFd := syscall.Handle(ln.Fd())

	// set SO_UPDATE_ACCEPT_CONTEXT
	setAcceptSocketOptErr := syscall.Setsockopt(
		sock,
		windows.SOL_SOCKET, windows.SO_UPDATE_ACCEPT_CONTEXT,
		(*byte)(unsafe.Pointer(&lnFd)),
		int32(unsafe.Sizeof(lnFd)),
	)
	if setAcceptSocketOptErr != nil {
		_ = syscall.Closesocket(sock)
		op.callback(Userdata{}, os.NewSyscallError("setsockopt", setAcceptSocketOptErr))
		return
	}

	// get local addr
	lsa, lsaErr := syscall.Getsockname(sock)
	if lsaErr != nil {
		_ = syscall.Closesocket(sock)
		op.callback(Userdata{}, os.NewSyscallError("getsockname", lsaErr))
		return
	}
	la := SockaddrToAddr(ln.Network(), lsa)

	// get remote addr
	rsa, rsaErr := syscall.Getpeername(sock)
	if rsaErr != nil {
		_ = syscall.Closesocket(sock)
		op.callback(Userdata{}, os.NewSyscallError("getsockname", rsaErr))
		return
	}
	ra := SockaddrToAddr(ln.Network(), rsa)

	// create iocp
	iocpErr := createSubIoCompletionPort(windows.Handle(sock))
	if iocpErr != nil {
		_ = syscall.Closesocket(sock)
		op.callback(Userdata{}, iocpErr)
		return
	}

	conn := &netFd{
		handle:     op.handle,
		network:    ln.Network(),
		family:     ln.Family(),
		socketType: ln.SocketType(),
		protocol:   ln.Protocol(),
		ipv6only:   ln.IPv6Only(),
		localAddr:  la,
		remoteAddr: ra,
		rop:        nil,
		wop:        nil,
	}
	conn.rop = newOperator(conn, ReadOperator)
	conn.wop = newOperator(conn, WriteOperator)

	// callback
	op.callback(Userdata{Fd: conn}, err)
	return
}
