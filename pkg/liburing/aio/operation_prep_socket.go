//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/liburing"
	"unsafe"
)

func (op *Operation) PrepareSocket(family int, sotype int, proto int) {
	op.code = liburing.OpSocket
	op.fd = family
	op.addr = unsafe.Pointer(uintptr(sotype))
	op.addrLen = uint32(proto)
	return
}

func (op *Operation) packingSocket(sqe *liburing.SubmissionQueueEntry) (err error) {
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
		op.sqeFlags |= liburing.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= liburing.SQEAsync
	}
	op.code = liburing.OpUringCmd
	op.cmd = liburing.SocketOpSetsockopt

	op.fd = fd
	op.addr = unsafe.Pointer(uintptr(level))
	op.addrLen = uint32(optName)
	op.addr2 = unsafe.Pointer(optValue)
	return
}

func (op *Operation) packingSetSocketoptInt(sqe *liburing.SubmissionQueueEntry) (err error) {
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
		op.sqeFlags |= liburing.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= liburing.SQEAsync
	}

	op.code = liburing.OpUringCmd
	op.cmd = liburing.SocketOpGetsockopt

	op.fd = fd
	op.addr = unsafe.Pointer(uintptr(level))
	op.addrLen = uint32(optName)
	op.addr2 = unsafe.Pointer(optValue)
	return
}

func (op *Operation) packingGetSocketoptInt(sqe *liburing.SubmissionQueueEntry) (err error) {
	fd := op.fd
	level := int(uintptr(op.addr))
	optName := int(op.addrLen)
	optValue := (*int)(op.addr2)

	sqe.PrepareGetsockoptInt(fd, level, optName, optValue)
	sqe.SetFlags(op.sqeFlags)
	sqe.SetData(unsafe.Pointer(op))
	return err
}
