//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/iouring"
	"unsafe"
)

func (op *Operation) PrepareSocket(family int, sotype int, proto int) {
	op.code = iouring.OpSocket
	op.pipe.fdIn = family
	op.pipe.fdOut = sotype
	op.pipe.offIn = int64(proto)
	return
}

func (op *Operation) PrepareSetSocketoptInt(nfd *NetFd, level int, optName int, optValue int) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= iouring.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= iouring.SQEAsync
	}
	op.code = iouring.OpUringCmd
	op.subKind = iouring.SocketOpSetsockopt
	op.fd = fd
	op.pipe.fdIn = level
	op.pipe.fdOut = optName
	op.pipe.offIn = int64(optValue)
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
	op.subKind = iouring.SocketOpGetsockopt
	op.fd = fd
	op.pipe.fdIn = level
	op.pipe.fdOut = optName
	op.ptr = unsafe.Pointer(optValue)
	return
}
