//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/iouring"
)

func (op *Operation) PrepareRead(nfd *Fd, b []byte) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= iouring.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= iouring.SQEAsync
	}
	op.kind = iouring.OpRead
	op.fd = fd
	op.msg.Name = &b[0]
	op.msg.Namelen = uint32(len(b))
	return
}

func (op *Operation) PrepareWrite(nfd *Fd, b []byte) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= iouring.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= iouring.SQEAsync
	}
	op.kind = iouring.OpWrite
	op.fd = fd
	op.msg.Name = &b[0]
	op.msg.Namelen = uint32(len(b))
	return
}

func (op *Operation) PrepareReadFixed(nfd *Fd, buf *FixedBuffer) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= iouring.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= iouring.SQEAsync
	}
	op.kind = iouring.OpReadFixed
	op.fd = fd
	op.msg.Name = &buf.value[buf.rPos]
	op.msg.Namelen = uint32(len(buf.value) - buf.rPos)
	op.msg.Iovlen = uint64(buf.index)
	return
}

func (op *Operation) PrepareWriteFixed(nfd *Fd, buf *FixedBuffer) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= iouring.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= iouring.SQEAsync
	}
	op.kind = iouring.OpWriteFixed
	op.fd = fd
	op.msg.Name = &buf.value[buf.rPos]
	op.msg.Namelen = uint32(buf.Length())
	op.msg.Iovlen = uint64(buf.index)
	return
}

func (op *Operation) PrepareSplice(fdIn int, offIn int64, fdOut int, offOut int64, nbytes uint32, flags uint32) {
	op.kind = iouring.OpSplice
	op.pipe.fdIn = fdIn
	op.pipe.offIn = offIn
	op.pipe.fdOut = fdOut
	op.pipe.offOut = offOut
	op.pipe.nbytes = nbytes
	op.pipe.spliceFlags = flags
}

func (op *Operation) PrepareTee(fdIn int, fdOut int, nbytes uint32, flags uint32) {
	op.kind = iouring.OpTee
	op.pipe.fdIn = fdIn
	op.pipe.fdOut = fdOut
	op.pipe.nbytes = nbytes
	op.pipe.spliceFlags = flags
}
