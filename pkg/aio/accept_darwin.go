//go:build darwin

package aio

import (
	"errors"
	"os"
	"runtime"
	"syscall"
)

func Accept(fd NetFd, cb OperationCallback) {
	op := fd.ReadOperator()
	op.callback = cb
	op.completion = completeAccept

	cylinder := nextKqueueCylinder()
	op.setCylinder(cylinder)

	if err := cylinder.prepareRead(fd.Fd(), op); err != nil {
		cb(Userdata{}, err)
		// reset
		op.reset()
	}
}

func completeAccept(result int, op *Operator, err error) {
	cb := op.callback
	if err != nil {
		cb(Userdata{}, err)
		return
	}
	if result == 0 {
		cb(Userdata{}, ErrBusy)
		return
	}
	ln := op.fd.(NetFd)
	sock := 0
	var sa syscall.Sockaddr
	for {
		sock, sa, err = syscall.Accept(ln.Fd())
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
	la := SockaddrToAddr(ln.Network(), lsa)
	// get remote addr
	ra := SockaddrToAddr(ln.Network(), sa)

	// conn
	conn := &netFd{
		handle:     sock,
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
	conn.rop = newOperator(conn, readOperator)
	conn.wop = newOperator(conn, writeOperator)

	cb(Userdata{Fd: conn}, nil)
	runtime.KeepAlive(ln)
	return
}
