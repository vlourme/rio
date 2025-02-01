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
		processing: false,
		cylinder:   nil,
		fd:         fd,
		handle:     -1,
		n:          0,
		oobn:       0,
		rsa:        nil,
		msg:        nil,
		sfr:        nil,
		callback:   nil,
		completion: nil,
	}
}

type Operator struct {
	processing bool
	cylinder   *IOURingCylinder
	fd         Fd
	handle     int
	n          uint32
	oobn       uint32
	rsa        *syscall.RawSockaddrAny
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
	if op.processing {
		op.processing = false
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
	if op.oobn != 0 {
		op.oobn = 0
	}
	if op.rsa != nil {
		op.rsa = nil
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
