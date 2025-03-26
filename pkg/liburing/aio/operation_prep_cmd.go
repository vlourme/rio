//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
)

func (op *Operation) packingURingCMD(sqe *liburing.SubmissionQueueEntry) (err error) {
	switch op.cmd {
	case liburing.SOCKET_URING_OP_SETSOCKOPT:
		err = op.packingSetSocketoptInt(sqe)
		break
	case liburing.SOCKET_URING_OP_GETSOCKOPT:
		err = op.packingGetSocketoptInt(sqe)
		break
	default:
		err = NewInvalidOpErr(errors.New("unsupported iouring command"))
		return
	}
	return
}
