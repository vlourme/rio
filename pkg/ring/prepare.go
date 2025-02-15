package ring

import (
	"context"
	"github.com/brickingsoft/errors"
	"github.com/pawelgaczynski/giouring"
	"runtime"
	"unsafe"
)

type Result struct {
	N     int
	Flags uint32
	Err   error
}

// Await
// when err is ErrUncompleted
func (ring *Ring) Await(ctx context.Context, op *Operation) (n int, hijacked bool, err error) {
	ch := op.ch
	if timeout := op.timeout; timeout > 0 {
		timer := ring.acquireTimer(timeout)

		select {
		case r := <-ch:
			n, err = r.N, r.Err
			hijacked = r.Flags&giouring.CQEFMore != 0
			break
		case <-timer.C:
			if op.done.CompareAndSwap(false, true) {
				hijacked = true
				err = errors.From(ErrUncompleted, errors.WithWrap(ErrTimeout))
			} else {
				// result has been sent, so continue to fetch result
				r := <-ch
				n, err = r.N, r.Err
				hijacked = r.Flags&giouring.CQEFMore != 0
			}
			break
		case <-ctx.Done():
			if op.done.CompareAndSwap(false, true) {
				hijacked = true
				err = errors.From(ErrUncompleted, errors.WithWrap(ctx.Err()))
			} else {
				r := <-ch
				n, err = r.N, r.Err
				hijacked = r.Flags&giouring.CQEFMore != 0
			}
			break
		}
		ring.releaseTimer(timer)
	} else {
		select {
		case r := <-ch:
			n, err = r.N, r.Err
			hijacked = r.Flags&giouring.CQEFMore != 0
			break
		case <-ctx.Done():
			if op.done.CompareAndSwap(false, true) {
				hijacked = true
				err = errors.From(ErrUncompleted, errors.WithWrap(ctx.Err()))
			} else {
				r := <-ch
				n, err = r.N, r.Err
				hijacked = r.Flags&giouring.CQEFMore != 0
			}
			break
		}
	}
	if hijacked {
		ring.hijackedOps.Store(op, struct{}{})
	}
	return
}

func (ring *Ring) Accept(ctx context.Context, fd int) (n int, err error) {
	op := ring.AcquireOperation()
	op.PrepareAccept(fd)
	if pushErr := ring.Push(op); pushErr != nil {
		ring.ReleaseOperation(op)
		err = pushErr // todo make err
		return
	}
	hijacked := false
	n, hijacked, err = ring.Await(ctx, op)
	if !hijacked {
		ring.ReleaseOperation(op)
	}
	return
}

func (ring *Ring) Receive(ctx context.Context, fd int, b []byte) (n int, err error) {
	op := ring.AcquireOperation()
	op.PrepareReceive(fd, b)
	for i := 0; i < 10; i++ {
		if err = ring.Push(op); err != nil {
			continue
		}
		break
	}
	if err != nil {
		// todo make err
		ring.ReleaseOperation(op)
		return
	}
	hijacked := false
	n, hijacked, err = ring.Await(ctx, op)
	if !hijacked {
		ring.ReleaseOperation(op)
	}
	return
}

func (ring *Ring) Send(ctx context.Context, fd int, b []byte) (n int, err error) {
	op := ring.AcquireOperation()
	op.PrepareSend(fd, b)
	for i := 0; i < 10; i++ {
		if err = ring.Push(op); err != nil {
			continue
		}
		break
	}
	if err != nil {
		// todo make err
		ring.ReleaseOperation(op)
		return
	}
	hijacked := false
	n, hijacked, err = ring.Await(ctx, op)
	if !hijacked {
		ring.ReleaseOperation(op)
	}
	return
}

func (ring *Ring) prepare(op *Operation) (bool, error) {
	sqe := ring.ring.GetSQE()
	if sqe == nil {
		return false, nil
	}
	switch op.kind {
	case nop:
		sqe.PrepareNop()
		sqe.SetData(unsafe.Pointer(op))
		break
	case acceptOp:
		addrPtr := uintptr(unsafe.Pointer(op.msg.Name))
		addrLenPtr := uint64(uintptr(unsafe.Pointer(&op.msg.Namelen)))
		sqe.PrepareAccept(op.fd, addrPtr, addrLenPtr, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case receiveOp:
		b := uintptr(unsafe.Pointer(&op.b[0]))
		bLen := uint32(len(op.b))
		sqe.PrepareRecv(op.fd, b, bLen, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case sendOp:
		b := uintptr(unsafe.Pointer(&op.b[0]))
		bLen := uint32(len(op.b))
		sqe.PrepareSend(op.fd, b, bLen, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case sendZCOp:
		sqe.PrepareSendZC(op.fd, op.b, 0, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case receiveFromOp, receiveMsgOp:
		msg := op.msg
		sqe.PrepareRecvMsg(op.fd, &msg, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case sendToOp, sendMsgOp:
		msg := op.msg
		sqe.PrepareSendMsg(op.fd, &msg, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case sendMsgZcOp:
		msg := op.msg
		sqe.PrepareSendmsgZC(op.fd, &msg, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case spliceOp:
		sp := op.splice
		sqe.PrepareSplice(sp.fdIn, sp.offIn, sp.fdOut, sp.offOut, sp.nbytes, sp.spliceFlags)
		sqe.SetData(unsafe.Pointer(op))
		break
	case teeOp:
		sp := op.splice
		sqe.PrepareTee(sp.fdIn, sp.fdOut, sp.nbytes, sp.spliceFlags)
		sqe.SetData(unsafe.Pointer(op))
		break
	default:
		sqe.PrepareNop()
		return false, errors.New("unknown op") // todo make err
	}
	runtime.KeepAlive(sqe)
	return true, nil
}
