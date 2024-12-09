//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

import (
	"os"
	"syscall"
)

func Accept(fd NetFd, cb OperationCallback) {
	sock, sa, accpetErr := syscall.Accept(fd.Fd())
	if accpetErr != nil {
		cb(0, Userdata{}, os.NewSyscallError("accept", accpetErr))
		return
	}
	// get local addr
	lsa, lsaErr := syscall.Getsockname(sock)
	if lsaErr != nil {
		_ = syscall.Close(sock)
		cb(0, Userdata{}, os.NewSyscallError("getsockname", lsaErr))
		return
	}
	la := SockaddrToAddr(fd.Network(), lsa)

	ra := SockaddrToAddr(fd.Network(), sa)

	op := ReadOperator(fd)
	userdata := op.userdata
	// conn
	conn := &netFd{
		handle:     sock,
		network:    fd.Network(),
		family:     fd.Family(),
		socketType: fd.SocketType(),
		protocol:   fd.Protocol(),
		localAddr:  la,
		remoteAddr: ra,
		rop:        Operator{},
		wop:        Operator{},
	}
	conn.rop.fd = conn
	conn.wop.fd = conn

	userdata.Fd = conn
	// cb
	cb(sock, userdata, nil)
}

var (
	somaxconn = syscall.SOMAXCONN
)
