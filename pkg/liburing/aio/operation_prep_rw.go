//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/liburing"
	"unsafe"
)

func (op *Operation) PrepareRead(nfd *Fd, b []byte) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.IOSQE_FIXED_FILE
	}
	if nfd.Async() {
		op.sqeFlags |= liburing.IOSQE_ASYNC
	}
	op.code = liburing.IORING_OP_READ
	op.fd = fd
	op.addr = unsafe.Pointer(&b[0])
	op.addrLen = uint32(len(b))
	return
}

func (op *Operation) packingRead(sqe *liburing.SubmissionQueueEntry) (err error) {
	b := uintptr(op.addr)
	bLen := op.addrLen
	sqe.PrepareRead(op.fd, b, bLen, 0)
	sqe.SetFlags(op.sqeFlags)
	sqe.SetData(unsafe.Pointer(op))
	return
}

func (op *Operation) PrepareWrite(nfd *Fd, b []byte) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.IOSQE_FIXED_FILE
	}
	if nfd.Async() {
		op.sqeFlags |= liburing.IOSQE_ASYNC
	}
	op.code = liburing.IORING_OP_WRITE
	op.fd = fd
	op.addr = unsafe.Pointer(&b[0])
	op.addrLen = uint32(len(b))
	return
}

func (op *Operation) packingWrite(sqe *liburing.SubmissionQueueEntry) (err error) {
	b := uintptr(op.addr)
	bLen := op.addrLen
	sqe.PrepareWrite(op.fd, b, bLen, 0)
	sqe.SetFlags(op.sqeFlags)
	sqe.SetData(unsafe.Pointer(op))
	return
}

func (op *Operation) PrepareReadFixed(nfd *Fd, buf *FixedBuffer) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.IOSQE_FIXED_FILE
	}
	if nfd.Async() {
		op.sqeFlags |= liburing.IOSQE_ASYNC
	}
	op.code = liburing.IORING_OP_READ_FIXED
	op.fd = fd
	op.addr = unsafe.Pointer(buf)
	return
}

func (op *Operation) packingReadFixed(sqe *liburing.SubmissionQueueEntry) (err error) {
	buf := (*FixedBuffer)(op.addr)
	b := uintptr(unsafe.Pointer(&buf.value[buf.rPos]))
	bLen := uint32(len(buf.value) - buf.rPos)
	idx := uint64(buf.index)
	sqe.PrepareReadFixed(op.fd, b, bLen, 0, int(idx))
	sqe.SetFlags(op.sqeFlags)
	sqe.SetData(unsafe.Pointer(op))
	return
}

func (op *Operation) PrepareWriteFixed(nfd *Fd, buf *FixedBuffer) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.IOSQE_FIXED_FILE
	}
	if nfd.Async() {
		op.sqeFlags |= liburing.IOSQE_ASYNC
	}
	op.code = liburing.IORING_OP_WRITE_FIXED
	op.fd = fd
	op.addr = unsafe.Pointer(buf)
	return
}

func (op *Operation) packingWriteFixed(sqe *liburing.SubmissionQueueEntry) (err error) {
	buf := (*FixedBuffer)(op.addr)
	b := uintptr(unsafe.Pointer(&buf.value[buf.rPos]))
	bLen := uint32(buf.Length())
	idx := uint64(buf.index)
	sqe.PrepareWriteFixed(op.fd, b, bLen, 0, int(idx))
	sqe.SetFlags(op.sqeFlags)
	sqe.SetData(unsafe.Pointer(op))
	return
}

type SpliceParams struct {
	FdIn       int
	FdInFixed  bool
	OffIn      int64
	FdOut      int
	FdOutFixed bool
	OffOut     int64
	NBytes     uint32
	Flags      uint32
}

func (op *Operation) PrepareSplice(params *SpliceParams) {
	op.code = liburing.IORING_OP_SPLICE
	if params.FdInFixed {
		params.Flags |= liburing.SPLICE_F_FD_IN_FIXED
	}
	op.addr = unsafe.Pointer(params)
	if params.FdOutFixed {
		op.sqeFlags |= liburing.IOSQE_FIXED_FILE
	}
}

func (op *Operation) packingSplice(sqe *liburing.SubmissionQueueEntry) (err error) {
	params := (*SpliceParams)(op.addr)
	sqe.PrepareSplice(params.FdIn, params.OffIn, params.FdOut, params.OffOut, params.NBytes, params.Flags)
	sqe.SetFlags(op.sqeFlags)
	sqe.SetData(unsafe.Pointer(op))
	return
}
