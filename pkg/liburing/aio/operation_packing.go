//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"runtime"
)

func (op *Operation) packingSQE(sqe *liburing.SubmissionQueueEntry) (err error) {
	switch op.code {
	case liburing.OpNop:
		err = op.packingNop(sqe)
		break
	case liburing.OpUringCmd:
		err = op.packingURingCMD(sqe)
		break
	case liburing.OpAsyncCancel:
		err = op.packingCancel(sqe)
		break
	case liburing.OpClose:
		err = op.packingClose(sqe)
		break
	case liburing.OPFixedFdInstall:
		err = op.packingFixedFdInstall(sqe)
		break
	case liburing.OpShutdown:
		err = op.packingShutdown(sqe)
		break
	case liburing.OpLinkTimeout:
		err = op.packingLinkTimeout(sqe)
		break
	case liburing.OpRead:
		err = op.packingRead(sqe)
		break
	case liburing.OpWrite:
		err = op.packingWrite(sqe)
		break
	case liburing.OpReadFixed:
		err = op.packingReadFixed(sqe)
		break
	case liburing.OpWriteFixed:
		err = op.packingWriteFixed(sqe)
		break
	case liburing.OpSocket:
		err = op.packingSocket(sqe)
		break
	case liburing.OpConnect:
		err = op.packingConnect(sqe)
		break
	case liburing.OpListen:
		err = op.packingListen(sqe)
		break
	case liburing.OpBind:
		err = op.packingBind(sqe)
		break
	case liburing.OpAccept:
		err = op.packingAccept(sqe)
		break
	case liburing.OpRecv:
		err = op.packingReceive(sqe)
		break
	case liburing.OpSend:
		err = op.packingSend(sqe)
		break
	case liburing.OpSendZC:
		err = op.packingSendZC(sqe)
		break
	case liburing.OpRecvmsg:
		err = op.packingReceiveMsg(sqe)
		break
	case liburing.OpSendmsg:
		err = op.packingSendMsg(sqe)
		break
	case liburing.OpSendMsgZC:
		err = op.packingSendMsgZc(sqe)
		break
	case liburing.OpSplice:
		err = op.packingSplice(sqe)
		break
	default:
		return NewInvalidOpErr(errors.New("unsupported"))
	}
	runtime.KeepAlive(sqe)
	return
}
