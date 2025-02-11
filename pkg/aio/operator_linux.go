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

type Operator struct {
	cqeFlags   uint32
	fd         Fd
	handle     int
	n          uint32
	b          []byte
	msg        syscall.Msghdr
	sfr        SendfileResult
	callback   OperationCallback
	completion OperatorCompletion
}

func (op *Operator) setFd(fd Fd) {
	op.fd = fd
}

func (op *Operator) reset() {
	op.cqeFlags = 0

	op.fd = nil
	op.handle = 0

	op.n = 0
	op.b = nil
	op.msg.Name = nil
	op.msg.Namelen = 0
	op.msg.Iov = nil
	op.msg.Iovlen = 0
	op.msg.Control = nil
	op.msg.Controllen = 0
	op.msg.Flags = 0

	op.sfr.file = -1
	op.sfr.remain = 0
	op.sfr.pipe = nil

	op.callback = nil
	op.completion = nil
	return
}

func (op *Operator) ptr() uint64 {
	return uint64(uintptr(unsafe.Pointer(op)))
}
