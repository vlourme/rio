//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

import "syscall"

func Cancel(op *Operator) {
	switch op.kind {
	case ReadOperator:
		cylinder := op.cylinder
		fd := op.fd.Fd()
		_ = cylinder.prepareRW(fd, syscall.EVFILT_READ, syscall.EV_DELETE|syscall.EV_ONESHOT|syscall.EV_CLEAR, nil)
		break
	case WriteOperator:
		cylinder := op.cylinder
		fd := op.fd.Fd()
		_ = cylinder.prepareRW(fd, syscall.EVFILT_WRITE, syscall.EV_DELETE|syscall.EV_ONESHOT|syscall.EV_CLEAR, nil)
		break
	default:
		break
	}
}
