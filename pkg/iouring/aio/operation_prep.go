//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"unsafe"
)

func (op *Operation) PrepareNop() (err error) {
	op.code = iouring.OpNop
	return
}

func (op *Operation) packingNop(sqe *iouring.SubmissionQueueEntry) (err error) {
	sqe.PrepareNop()
	return
}

func (op *Operation) prepareLinkTimeout(target *Operation) {
	op.code = iouring.OpLinkTimeout
	op.timeout = target.timeout
	target.target = op
}

func (op *Operation) getLinkTimeoutOp() *Operation {
	return op.target
}

func (op *Operation) packingLinkTimeout(sqe *iouring.SubmissionQueueEntry) (err error) {
	if op.timeout == nil {
		return NewInvalidOpErr(errors.New("invalid timeout"))
	}
	sqe.PrepareLinkTimeout(op.timeout, 0)
	sqe.SetData(unsafe.Pointer(op))
	return
}
