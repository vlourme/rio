//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

import (
	"syscall"
)

type operatorKind int

const (
	readOperator operatorKind = iota + 1
	writeOperator
)

type SendfileResult struct {
	file    int
	curpos  int64
	remain  int64
	written int
}

func newOperator(fd Fd, kind operatorKind) *Operator {
	return &Operator{
		kind:   kind,
		fd:     fd,
		handle: -1,
	}
}

type Operator struct {
	kind       operatorKind
	fd         Fd
	processing bool
	cylinder   *KqueueCylinder
	handle     int
	b          []byte
	n          uint32
	oob        []byte
	oobn       uint32
	sa         syscall.Sockaddr
	sfr        *SendfileResult
	callback   OperationCallback
	completion OperatorCompletion
}

func (op *Operator) setCylinder(cylinder *KqueueCylinder) {
	op.cylinder = cylinder
	op.processing = true
}

func (op *Operator) Processing() bool {
	return op.processing
}

func (op *Operator) end() {
	op.processing = false
}

func (op *Operator) reset() {
	if op.processing {
		op.processing = false
	}
	if op.cylinder != nil {
		op.cylinder = nil
	}
	if op.handle != -1 {
		op.handle = -1
	}
	if op.b != nil {
		op.b = nil
	}
	if op.n != 0 {
		op.n = 0
	}
	if op.oob != nil {
		op.oob = nil
	}
	if op.oobn != 0 {
		op.oobn = 0
	}
	if op.sa != nil {
		op.sa = nil
	}
	if op.sfr != nil {
		op.sfr = nil
	}
	if op.callback != nil {
		op.callback = nil
	}
	if op.completion != nil {
		op.completion = nil
	}
}
