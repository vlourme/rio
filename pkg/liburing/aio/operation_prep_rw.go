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
