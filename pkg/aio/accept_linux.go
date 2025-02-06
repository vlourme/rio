//go:build linux

package aio

import (
	"github.com/brickingsoft/errors"
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
	op := acquireOperator(fd)
	// cb
	op.callback = cb
	// completion
	op.completion = completeAccept

	// cylinder
	cylinder := nextIOURingCylinder()
	op.setCylinder(cylinder)

	// addr
	addrPtr := uintptr(unsafe.Pointer(new(syscall.RawSockaddrAny)))
	addrLen := syscall.SizeofSockaddrAny
	addrLenPtr := uint64(uintptr(unsafe.Pointer(&addrLen)))

	// ln
	sock := fd.Fd()
	// prepare
	err := cylinder.prepareRW(opAccept, sock, addrPtr, 0, addrLenPtr, 0, op.ptr())
	if err != nil {
		err = errors.New(
			"accept failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpAccept),
			errors.WithWrap(os.NewSyscallError("io_uring_prep_accept", err)),
		)
		cb(Userdata{}, err)
		// release
		releaseOperator(op)
	}
	return
}

func completeAccept(result int, op *Operator, err error) {
	cb := op.callback
	fd := op.fd.(NetFd)

	// release
	releaseOperator(op)

	if err != nil {
		err = errors.New(
			"accept failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpAccept),
			errors.WithWrap(err),
		)
		cb(Userdata{}, err)
		return
	}
	// conn
	sock := result
	// get local addr
	lsa, lsaErr := syscall.Getsockname(sock)
	if lsaErr != nil {
		_ = syscall.Close(sock)
		err = errors.New(
			"accept failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpAccept),
			errors.WithWrap(os.NewSyscallError("getsockname", lsaErr)),
		)
		cb(Userdata{}, err)
		return
	}
	la := SockaddrToAddr(fd.Network(), lsa)

	// get remote addr
	rsa, rsaErr := syscall.Getpeername(sock)
	if rsaErr != nil {
		_ = syscall.Close(sock)
		err = errors.New(
			"accept failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpAccept),
			errors.WithWrap(os.NewSyscallError("getpeername", rsaErr)),
		)
		cb(Userdata{}, err)
		return
	}
	ra := SockaddrToAddr(fd.Network(), rsa)

	// conn
	conn := newNetFd(sock, fd.Network(), fd.Family(), fd.SocketType(), fd.Protocol(), fd.IPv6Only(), la, ra)
	// cb
	cb(Userdata{Fd: conn}, nil)
	return
}
