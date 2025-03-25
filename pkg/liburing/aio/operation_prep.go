//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"unsafe"
)

func (op *Operation) PrepareNop() (err error) {
	op.code = liburing.OpNop
	return
}

func (op *Operation) packingNop(sqe *liburing.SubmissionQueueEntry) (err error) {
	sqe.PrepareNop()
	return
}

func (op *Operation) prepareLinkTimeout(target *Operation) {
	op.code = liburing.OpLinkTimeout
	op.timeout = target.timeout
	op.status.Store(ProcessingOperationStatus)
	target.addr2 = unsafe.Pointer(op)
}

func (op *Operation) packingLinkTimeout(sqe *liburing.SubmissionQueueEntry) (err error) {
	if op.timeout == nil {
		return NewInvalidOpErr(errors.New("invalid timeout"))
	}
	sqe.PrepareLinkTimeout(op.timeout, 0)
	sqe.SetData(unsafe.Pointer(op))
	return
}
