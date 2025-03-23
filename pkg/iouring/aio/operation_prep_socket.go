//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/iouring"
	"unsafe"
)

func (op *Operation) PrepareSocket(family int, sotype int, proto int) {
	op.code = iouring.OpSocket
	op.fd = family
	op.addr = unsafe.Pointer(uintptr(sotype))
	op.addrLen = uint32(proto)
	return
}

func (op *Operation) packingSocket(sqe *iouring.SubmissionQueueEntry) (err error) {
	family := op.fd
	sotype := int(uintptr(op.addr))
	proto := int(op.addrLen)
	if op.flags&directFd != 0 {
		sqe.PrepareSocketDirectAlloc(family, sotype, proto, 0)
	} else {
		sqe.PrepareSocket(family, sotype, proto, 0)
	}
	sqe.SetData(unsafe.Pointer(op))
	return
}

func (op *Operation) PrepareSetSocketoptInt(nfd *NetFd, level int, optName int, optValue *int) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= iouring.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= iouring.SQEAsync
	}
	op.code = iouring.OpUringCmd
	op.cmd = iouring.SocketOpSetsockopt

	op.fd = fd
	op.addr = unsafe.Pointer(uintptr(level))
	op.addrLen = uint32(optName)
	op.addr2 = unsafe.Pointer(optValue)
	return
}

func (op *Operation) packingSetSocketoptInt(sqe *iouring.SubmissionQueueEntry) (err error) {
	fd := op.fd
	level := int(uintptr(op.addr))
	optName := int(op.addrLen)
	optValue := (*int)(op.addr2)

	sqe.PrepareSetsockoptInt(fd, level, optName, optValue)
	sqe.SetFlags(op.sqeFlags)
	sqe.SetData(unsafe.Pointer(op))
	return
}

func (op *Operation) PrepareGetSocketoptInt(nfd *NetFd, level int, optName int, optValue *int) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= iouring.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= iouring.SQEAsync
	}

	op.code = iouring.OpUringCmd
	op.cmd = iouring.SocketOpGetsockopt

	op.fd = fd
	op.addr = unsafe.Pointer(uintptr(level))
	op.addrLen = uint32(optName)
	op.addr2 = unsafe.Pointer(optValue)
	return
}

func (op *Operation) packingGetSocketoptInt(sqe *iouring.SubmissionQueueEntry) (err error) {
	fd := op.fd
	level := int(uintptr(op.addr))
	optName := int(op.addrLen)
	optValue := (*int)(op.addr2)

	sqe.PrepareGetsockoptInt(fd, level, optName, optValue)
	sqe.SetFlags(op.sqeFlags)
	sqe.SetData(unsafe.Pointer(op))
	return err
}
