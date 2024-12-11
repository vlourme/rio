//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

import (
	"errors"
	"os"
	"runtime"
	"syscall"
)

func Accept(fd NetFd, cb OperationCallback) {
	op := ReadOperator(fd)
	op.userdata.Fd = fd
	op.callback = cb
	op.completion = func(result int, cop *Operator, err error) {
		completeAccept(result, cop, err)
		runtime.KeepAlive(op)
	}
	cylinder := nextKqueueCylinder()
	if err := cylinder.prepareRead(fd.Fd(), op); err != nil {
		cb(0, Userdata{}, err)
		// reset
		op.callback = nil
		op.completion = nil
	}
	return
}

func completeAccept(result int, cop *Operator, err error) {
	userdata := Userdata{}
	cb := cop.callback
	if err != nil {
		cb(0, userdata, err)
		return
	}

	ln := cop.userdata.Fd.(NetFd)
	if result == 0 {
		Accept(ln, cb)
		return
	}
	//TODO: 可能会堵住。测试堵是否会影响外部的投递。
	sock, sa, acceptErr := syscall.Accept(ln.Fd())
	runtime.KeepAlive(ln)
	if acceptErr != nil {
		if errors.Is(acceptErr, syscall.EAGAIN) {
			Accept(ln, cb)
			return
		}
		cb(0, userdata, os.NewSyscallError("accept", acceptErr))
		return
	}
	syscall.CloseOnExec(sock)
	if setNonblockErr := syscall.SetNonblock(sock, true); setNonblockErr != nil {
		cb(0, userdata, os.NewSyscallError("setnonblock", setNonblockErr))
		return
	}
	// get local addr
	lsa, lsaErr := syscall.Getsockname(sock)
	if lsaErr != nil {
		_ = syscall.Close(sock)
		cb(0, Userdata{}, os.NewSyscallError("getsockname", lsaErr))
		return
	}
	la := SockaddrToAddr(ln.Network(), lsa)

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
		rop:        Operator{},
		wop:        Operator{},
	}
	conn.rop.fd = conn
	conn.wop.fd = conn

	userdata.Fd = conn

	cb(sock, userdata, nil)
	return
}

var (
	somaxconn = maxListenerBacklog()
)

func maxListenerBacklog() int {
	var (
		n   uint32
		err error
	)
	switch runtime.GOOS {
	case "darwin":
		n, err = syscall.SysctlUint32("kern.ipc.somaxconn")
	case "freebsd":
		n, err = syscall.SysctlUint32("kern.ipc.soacceptqueue")
	}
	if n == 0 || err != nil {
		return syscall.SOMAXCONN
	}
	if n > 1<<16-1 {
		n = 1<<16 - 1
	}
	return int(n)
}
