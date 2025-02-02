//go:build linux

package aio

import (
	"syscall"
	"unsafe"
)

type SendfileResult struct {
	file   int
	remain uint32
	pipe   []int
}

func newOperator(fd Fd) *Operator {
	return &Operator{
		fd:     fd,
		handle: -1,
	}
}

type Operator struct {
	processing bool
	hijacked   bool
	cylinder   *IOURingCylinder
	cqeFlags   uint32
	fd         Fd
	handle     int
	n          uint32
	msg        *syscall.Msghdr
	sfr        *SendfileResult
	callback   OperationCallback
	completion OperatorCompletion
}

func (op *Operator) setCylinder(cylinder *IOURingCylinder) {
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
	if op.hijacked {
		return
	}
	if op.processing {
		op.processing = false
	}
	if op.cqeFlags != 0 {
		op.cqeFlags = 0
	}
	if op.cylinder != nil {
		op.cylinder = nil
	}
	if op.handle != -1 {
		op.handle = -1
	}
	if op.n != 0 {
		op.n = 0
	}
	if op.msg != nil {
		op.msg = nil
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

func (op *Operator) ptr() uint64 {
	return uint64(uintptr(unsafe.Pointer(op)))
}
