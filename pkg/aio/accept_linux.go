//go:build linux

package aio

import (
	"os"
	"syscall"
	"unsafe"
)

/*
Accept
https://sf-zhou.github.io/linux/io_uring_network_programming.html
one accpet multi cb
对 server 来说，需要监听一个地址，处理所有 incoming 的连接。
liburing 中提供了 io_uring_prep_multishot_accept 方法，
或者你可以手动给 sqe->ioprio 增加一个 IORING_ACCEPT_MULTISHOT 标记。
这样每当有新的连接进来时，都会产生一个新的 CQE，返回的 flags 中会带有 IORING_CQE_F_MORE 标记。
如果希望优雅的退出，可以使用 io_uring_prep_cancel 或者 io_uring_prep_cancel_fd 取消掉监听操作。
*/
func Accept(fd NetFd, cb OperationCallback) {
	// op
	op := fd.ReadOperator()
	// msg
	addrPtr := uintptr(unsafe.Pointer(new(syscall.RawSockaddrAny)))
	addrLen := syscall.SizeofSockaddrAny
	addrLenPtr := uint64(uintptr(unsafe.Pointer(&addrLen)))

	// cb
	op.callback = cb
	// completion
	op.completion = completeAccept

	// cylinder
	cylinder := nextIOURingCylinder()
	op.setCylinder(cylinder)

	// ln
	lnFd := fd.Fd()
	// prepare
	err := cylinder.prepareRW(opAccept, lnFd, addrPtr, 0, addrLenPtr, 0, op.ptr())
	if err != nil {
		cb(Userdata{}, os.NewSyscallError("io_uring_prep_accept", err))
		// reset
		op.reset()
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
