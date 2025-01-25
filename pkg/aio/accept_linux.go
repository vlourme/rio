//go:build linux

package aio

import (
	"os"
	"syscall"
	"unsafe"
)

func Accept(fd NetFd, cb OperationCallback) {
	// op
	op := fd.ReadOperator()
	// msg
	rsa, rsaLen := op.msg.BuildRawSockaddrAny()
	addrPtr := uintptr(unsafe.Pointer(rsa))
	addrLenPtr := uint64(uintptr(unsafe.Pointer(&rsaLen)))

	// cb
	op.callback = cb
	// completion
	op.completion = completeAccept

	// cylinder
	cylinder := nextIOURingCylinder()

	// ln
	lnFd := fd.Fd()
	// prepare
	err := cylinder.prepare(opAccept, lnFd, addrPtr, 0, addrLenPtr, 0, op)
	if err != nil {
		cb(Userdata{}, os.NewSyscallError("io_uring_prep_accept", err))
		// clean
		op.clean()
	}
	return
}

func completeAccept(result int, op *Operator, err error) {
	// cb
	cb := op.callback
	if err != nil {
		cb(Userdata{}, os.NewSyscallError("io_uring_prep_accept", err))
		return
	}
	// conn
	connFd := result
	// ln
	ln, _ := op.fd.(NetFd)
	// addr
	// get local addr
	lsa, lsaErr := syscall.Getsockname(connFd)
	if lsaErr != nil {
		_ = syscall.Close(connFd)
		op.callback(Userdata{}, os.NewSyscallError("getsockname", lsaErr))
		return
	}
	la := SockaddrToAddr(ln.Network(), lsa)

	// get remote addr
	rsa, rsaErr := syscall.Getpeername(connFd)
	if rsaErr != nil {
		_ = syscall.Close(connFd)
		op.callback(Userdata{}, os.NewSyscallError("getpeername", rsaErr))
		return
	}
	ra := SockaddrToAddr(ln.Network(), rsa)

	// conn
	conn := &netFd{
		handle:     connFd,
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

	conn.rop = newOperator(conn)
	conn.wop = newOperator(conn)
	// cb
	cb(Userdata{Fd: conn}, nil)
	return
}
