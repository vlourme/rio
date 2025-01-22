//go:build linux

package aio

import (
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

func Accept(fd NetFd, cb OperationCallback) {
	// op
	op := ReadOperator(fd)
	// ln
	lnFd := fd.Fd()
	// msg
	rsa, rsaLen := op.userdata.Msg.BuildRawSockaddrAny()
	addrPtr := uintptr(unsafe.Pointer(rsa))
	addrLenPtr := uint64(uintptr(unsafe.Pointer(&rsaLen)))

	// cb
	op.callback = cb
	// completion
	op.completion = func(result int, cop *Operator, err error) {
		completeAccept(result, cop, err)
		runtime.KeepAlive(op)
	}

	// prepare
	err := prepare(opAccept, lnFd, addrPtr, 0, addrLenPtr, 0, op)
	runtime.KeepAlive(op)
	if err != nil {
		cb(-1, Userdata{}, os.NewSyscallError("io_uring_prep_accept", err))
		// reset
		op.callback = nil
		op.completion = nil
		return
	}
	return
}

func completeAccept(result int, op *Operator, err error) {
	// cb
	cb := op.callback
	// userdata
	userdata := op.userdata
	if err != nil {
		cb(-1, Userdata{}, os.NewSyscallError("io_uring_prep_accept", err))
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
		op.callback(-1, Userdata{}, os.NewSyscallError("getsockname", lsaErr))
		return
	}
	la := SockaddrToAddr(ln.Network(), lsa)

	// get remote addr
	rsa, rsaErr := syscall.Getpeername(connFd)
	if rsaErr != nil {
		_ = syscall.Close(connFd)
		op.callback(-1, Userdata{}, os.NewSyscallError("getpeername", rsaErr))
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
		rop:        Operator{},
		wop:        Operator{},
	}
	conn.rop.fd = conn
	conn.wop.fd = conn

	userdata.Fd = conn
	// cb
	cb(connFd, userdata, nil)
	return
}
