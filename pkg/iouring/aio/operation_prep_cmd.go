//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
)

func (op *Operation) packingURingCMD(sqe *iouring.SubmissionQueueEntry) (err error) {
	switch op.cmd {
	case iouring.SocketOpSetsockopt:
		err = op.packingSetSocketoptInt(sqe)
		break
	case iouring.SocketOpGetsockopt:
		err = op.packingGetSocketoptInt(sqe)
		break
	default:
		sqe.PrepareNop()
		err = NewInvalidOpErr(errors.New("unsupported iouring command"))
		return
	}
	return
}
