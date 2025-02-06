//go:build darwin

package aio

import (
	"errors"
	"os"
	"syscall"
)

func Accept(fd NetFd, cb OperationCallback) {
	op := acquireOperator(fd)
	op.callback = cb
	op.completion = completeAccept

	cylinder := nextKqueueCylinder()
	op.setCylinder(cylinder)

	if err := cylinder.prepareRead(fd.Fd(), op); err != nil {
		cb(Userdata{}, err)
		releaseOperator(op)
	}
}

func completeAccept(result int, op *Operator, err error) {
	cb := op.callback
	fd := op.fd.(NetFd)

	releaseOperator(op)
	if err != nil {
		cb(Userdata{}, err)
		return
	}
	if result == 0 {
		cb(Userdata{}, ErrBusy)
		return
	}

	sock := 0
	var sa syscall.Sockaddr
	for {
		sock, sa, err = syscall.Accept(fd.Fd())
		if err != nil {
			if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EINTR) || errors.Is(err, syscall.ECONNABORTED) {
				continue
			}
			cb(Userdata{}, os.NewSyscallError("accept4", err))
			return
		}
		break
	}
	syscall.CloseOnExec(sock)
	if setErr := syscall.SetNonblock(sock, true); setErr != nil {
		_ = syscall.Close(sock)
		cb(Userdata{}, os.NewSyscallError("setnonblock", setErr))
		return
	}

	// get local addr
	lsa, lsaErr := syscall.Getsockname(sock)
	if lsaErr != nil {
		_ = syscall.Close(sock)
		cb(Userdata{}, os.NewSyscallError("getsockname", lsaErr))
		return
	}
	la := SockaddrToAddr(fd.Network(), lsa)
	// get remote addr
	ra := SockaddrToAddr(fd.Network(), sa)

	// conn
	conn := newNetFd(sock, fd.Network(), fd.Family(), fd.SocketType(), fd.Protocol(), fd.IPv6Only(), la, ra)
	cb(Userdata{Fd: conn}, nil)
	return
}
