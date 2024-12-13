//go:build darwin

package aio

import (
	"errors"
	"os"
	"runtime"
	"syscall"
)

func Accept(fd NetFd, cb OperationCallback) {
	sock, sa, acceptErr := syscall.Accept(fd.Fd())
	runtime.KeepAlive(fd)
	if acceptErr != nil {
		if errors.Is(acceptErr, syscall.EAGAIN) || errors.Is(acceptErr, syscall.EINTR) || errors.Is(acceptErr, syscall.ECONNABORTED) {
			Accept(fd, cb)
			return
		}
		cb(0, Userdata{}, os.NewSyscallError("accept", acceptErr))
		return
	}

	syscall.CloseOnExec(sock)
	if setErr := syscall.SetNonblock(sock, true); setErr != nil {
		_ = syscall.Close(sock)
		cb(0, Userdata{}, os.NewSyscallError("setnonblock", setErr))
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
	// get remote addr
	ra := SockaddrToAddr(fd.Network(), sa)

	// conn
	conn := &netFd{
		handle:     sock,
		network:    fd.Network(),
		family:     fd.Family(),
		socketType: fd.SocketType(),
		protocol:   fd.Protocol(),
		ipv6only:   fd.IPv6Only(),
		localAddr:  la,
		remoteAddr: ra,
		rop:        Operator{},
		wop:        Operator{},
	}
	conn.rop.fd = conn
	conn.wop.fd = conn

	userdata := Userdata{
		Fd: conn,
	}

	cb(sock, userdata, nil)
	return
}
