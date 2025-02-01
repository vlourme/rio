//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

import (
	"runtime"
	"syscall"
)

func Cancel(op *Operator) {
	cylinder := op.cylinder
	fd := op.fd.Fd()
	switch op.kind {
	case readOperator:
		_ = cylinder.prepareRW(fd, syscall.EVFILT_READ, syscall.EV_DELETE, op)
	case writeOperator:
		fd := op.fd.Fd()
		_ = cylinder.prepareRW(fd, syscall.EVFILT_WRITE, syscall.EV_DELETE, op)
	default:
		return
	}
	runtime.KeepAlive(op)
}
