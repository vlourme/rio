//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

import (
	"syscall"
)

type SendfileResult struct {
	file    int
	curpos  int64
	remain  int64
	written int
}

type Operator struct {
	cylinder   *KqueueCylinder
	fd         Fd
	handle     int
	b          []byte
	n          uint32
	oob        []byte
	oobn       uint32
	sa         syscall.Sockaddr
	sfr        SendfileResult
	callback   OperationCallback
	completion OperatorCompletion
}

func (op *Operator) setFd(fd Fd) {
	op.fd = fd
}

func (op *Operator) setCylinder(cylinder *KqueueCylinder) {
	op.cylinder = cylinder
}

func (op *Operator) reset() {
	op.cylinder = nil
	op.fd = nil
	op.handle = -1
	op.b = nil
	op.n = 0
	op.oob = nil
	op.oobn = 0
	op.sa = nil
	op.sfr.file = 0
	op.sfr.curpos = 0
	op.sfr.remain = 0
	op.sfr.written = 0
	op.callback = nil
	op.completion = nil
	return
}
