//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"runtime"
)

func (op *Operation) packingSQE(sqe *iouring.SubmissionQueueEntry) (err error) {
	switch op.code {
	case iouring.OpNop:
		err = op.packingNop(sqe)
		break
	case iouring.OpUringCmd:
		err = op.packingURingCMD(sqe)
		break
	case iouring.OpAsyncCancel:
		err = op.packingCancel(sqe)
		break
	case iouring.OpClose:
		err = op.packingClose(sqe)
		break
	case iouring.OPFixedFdInstall:
		err = op.packingFixedFdInstall(sqe)
		break
	case iouring.OpShutdown:
		err = op.packingShutdown(sqe)
		break
	case iouring.OpLinkTimeout:
		err = op.packingLinkTimeout(sqe)
		break
	case iouring.OpRead:
		err = op.packingRead(sqe)
		break
	case iouring.OpWrite:
		err = op.packingWrite(sqe)
		break
	case iouring.OpReadFixed:
		err = op.packingReadFixed(sqe)
		break
	case iouring.OpWriteFixed:
		err = op.packingWriteFixed(sqe)
		break
	case iouring.OpSocket:
		err = op.packingSocket(sqe)
		break
	case iouring.OpConnect:
		err = op.packingConnect(sqe)
		break
	case iouring.OpListen:
		err = op.packingListen(sqe)
		break
	case iouring.OpBind:
		err = op.packingBind(sqe)
		break
	case iouring.OpAccept:
		err = op.packingAccept(sqe)
		break
	case iouring.OpRecv:
		err = op.packingReceive(sqe)
		break
	case iouring.OpSend:
		err = op.packingSend(sqe)
		break
	case iouring.OpSendZC:
		err = op.packingSendZC(sqe)
		break
	case iouring.OpRecvmsg:
		err = op.packingReceiveMsg(sqe)
		break
	case iouring.OpSendmsg:
		err = op.packingSendMsg(sqe)
		break
	case iouring.OpSendMsgZC:
		err = op.packingSendMsgZc(sqe)
		break
	case iouring.OpSplice:
		err = op.packingSplice(sqe)
		break
	default:
		return NewInvalidOpErr(errors.New("unsupported"))
	}
	runtime.KeepAlive(sqe)
	return
}
