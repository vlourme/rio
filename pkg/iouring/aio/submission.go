//go:build linux

package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"syscall"
	"time"
)

func (vortex *Vortex) Socket(ctx context.Context, family int, sotype int, proto int) (n int, err error) {
	op := vortex.acquireOperation()
	op.PrepareSocket(family, sotype, proto)
	n, _, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) SocketDirect(ctx context.Context, family int, sotype int, proto int) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithDirect(true).PrepareSocket(family, sotype, proto)
	n, _, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) SetSocketoptInt(ctx context.Context, fd int, level int, optName int, optValue int, sqeFlags uint8) (err error) {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).PrepareSetSocketoptInt(fd, level, optName, optValue)
	_, _, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) GetSocketoptInt(ctx context.Context, fd int, level int, optName int, sqeFlags uint8) (n int, err error) {
	var optValue int
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).PrepareGetSocketoptInt(fd, level, optName, &optValue)
	_, _, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	n = optValue
	return
}

func (vortex *Vortex) Connect(ctx context.Context, fd int, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time, sqeFlags uint8) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareConnect(fd, addr, addrLen)
	n, _, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) Accept(ctx context.Context, fd int, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time, sqeFlags uint8) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareAccept(fd, addr, addrLen)
	n, _, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) AcceptDirect(ctx context.Context, fd int, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time, sqeFlags uint8, fileIndex uint32) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).WithFiledIndex(fileIndex).WithDirect(true).PrepareAccept(fd, addr, addrLen)
	n, _, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) AcceptDirectAlloc(ctx context.Context, fd int, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time, sqeFlags uint8) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).WithFiledIndex(iouring.FileIndexAlloc).WithDirect(true).PrepareAccept(fd, addr, addrLen)
	n, _, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) Close(ctx context.Context, fd int) (err error) {
	op := vortex.acquireOperation()
	op.PrepareClose(fd)
	_, _, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) CloseDirect(ctx context.Context, fileIndex int) (err error) {
	op := vortex.acquireOperation()
	op.PrepareCloseDirect(fileIndex)
	_, _, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) ReadFixed(ctx context.Context, fd int, buf *FixedBuffer, deadline time.Time, sqeFlags uint8) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareReadFixed(fd, buf)
	n, _, err = vortex.submitAndWait(ctx, op)
	buf.rightShiftWritePosition(n)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) WriteFixed(ctx context.Context, fd int, buf *FixedBuffer, deadline time.Time, sqeFlags uint8) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareWriteFixed(fd, buf)
	n, _, err = vortex.submitAndWait(ctx, op)
	buf.rightShiftReadPosition(n)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) Receive(ctx context.Context, fd int, b []byte, deadline time.Time, sqeFlags uint8) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareReceive(fd, b)
	n, _, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) Send(ctx context.Context, fd int, b []byte, deadline time.Time, sqeFlags uint8) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareSend(fd, b)
	n, _, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) SendZC(ctx context.Context, fd int, b []byte, deadline time.Time, sqeFlags uint8) (n int, err error) {
	op := vortex.acquireOperation()
	op.Hijack()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareSendZC(fd, b)
	var (
		cqeFlags uint32
	)
	n, cqeFlags, err = vortex.submitAndWait(ctx, op)
	if err != nil {
		op.Complete()
		vortex.releaseOperation(op)
		return
	}

	if cqeFlags&iouring.CQEFMore != 0 {
		_, cqeFlags, err = vortex.awaitOperation(ctx, op)
		if err != nil {
			op.Complete()
			vortex.releaseOperation(op)
			return
		}
		if cqeFlags&iouring.CQEFNotify == 0 {
			err = errors.New("send_zc received CQE_F_MORE but no CQE_F_NOTIF")
		}
	}
	op.Complete()
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) ReceiveFrom(ctx context.Context, fd int, b []byte, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time, sqeFlags uint8) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareReceiveMsg(fd, b, nil, addr, addrLen, 0)
	n, _, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) SendTo(ctx context.Context, fd int, b []byte, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time, sqeFlags uint8) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareSendMsg(fd, b, nil, addr, addrLen, 0)
	n, _, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) SendToZC(ctx context.Context, fd int, b []byte, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time, sqeFlags uint8) (n int, err error) {
	n, _, err = vortex.SendMsgZC(ctx, fd, b, nil, addr, addrLen, deadline, sqeFlags)
	return
}

func (vortex *Vortex) ReceiveMsg(ctx context.Context, fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int, deadline time.Time, sqeFlags uint8) (n int, oobn int, flag int, err error) {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareReceiveMsg(fd, b, oob, addr, addrLen, int32(flags))
	n, _, err = vortex.submitAndWait(ctx, op)
	if err == nil {
		oobn = int(op.msg.Controllen)
		flag = int(op.msg.Flags)
	}
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) SendMsg(ctx context.Context, fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time, sqeFlags uint8) (n int, oobn int, err error) {
	op := vortex.acquireOperation()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareSendMsg(fd, b, oob, addr, addrLen, 0)
	n, _, err = vortex.submitAndWait(ctx, op)
	if err == nil {
		oobn = int(op.msg.Controllen)
	}
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) SendMsgZC(ctx context.Context, fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time, sqeFlags uint8) (n int, oobn int, err error) {
	op := vortex.acquireOperation()
	op.Hijack()
	op.WithSQEFlags(sqeFlags).WithDeadline(deadline).PrepareSendMsgZC(fd, b, oob, addr, addrLen, 0)
	var (
		cqeFlags uint32
	)
	n, cqeFlags, err = vortex.submitAndWait(ctx, op)
	if err != nil {
		op.Complete()
		vortex.releaseOperation(op)
		return
	}

	oobn = int(op.msg.Controllen)

	if cqeFlags&iouring.CQEFMore != 0 {
		_, cqeFlags, err = vortex.awaitOperation(ctx, op)
		if err != nil {
			op.Complete()
			vortex.releaseOperation(op)
			return
		}
		if cqeFlags&iouring.CQEFNotify == 0 {
			err = errors.New("sendmsg_zc received CQE_F_MORE but no CQE_F_NOTIF")
		}
	}
	op.Complete()
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) Cancel(ctx context.Context, target *Operation) (ok bool) {
	op := vortex.acquireOperation()
	op.PrepareCancel(target)
	_, _, err := vortex.submitAndWait(ctx, op)
	ok = err == nil
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) CancelFd(ctx context.Context, fd int) (ok bool) {
	op := vortex.acquireOperation()
	op.PrepareCancelFd(fd)
	_, _, err := vortex.submitAndWait(ctx, op)
	ok = err == nil
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) CancelFixedFd(ctx context.Context, fileIndex int) (ok bool) {
	op := vortex.acquireOperation()
	op.PrepareCancelFixedFd(fileIndex)
	_, _, err := vortex.submitAndWait(ctx, op)
	ok = err == nil
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) FixedFdInstall(ctx context.Context, fd int) (n int, err error) {
	op := vortex.acquireOperation()
	op.PrepareFixedFdInstall(fd)
	n, _, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) submitAndWait(ctx context.Context, op *Operation) (n int, cqeFlags uint32, err error) {
	deadline := op.deadline
RETRY:
	vortex.submit(op)
	n, cqeFlags, err = vortex.awaitOperation(ctx, op)
	if err != nil {
		if errors.Is(err, syscall.EBUSY) {
			if !deadline.IsZero() && deadline.Before(time.Now()) {
				err = ErrTimeout
				return
			}
			goto RETRY
		}
		return
	}
	return
}

func (vortex *Vortex) awaitOperation(ctx context.Context, op *Operation) (n int, cqeFlags uint32, err error) {
	var (
		done                    = ctx.Done()
		timerC <-chan time.Time = nil
	)
	timeout := op.Timeout()
	if timeout > 0 {
		timer := vortex.acquireTimer(timeout)
		defer vortex.releaseTimer(timer)
		timerC = timer.C
	} else if timeout < 0 {
		n, cqeFlags, err = vortex.cancelOperation(ctx, op)
		return
	}
	select {
	case r, ok := <-op.resultCh:
		if !ok {
			op.Close()
			err = ErrCanceled
			break
		}
		n, cqeFlags, err = r.N, r.Flags, r.Err
		if errors.Is(err, syscall.ECANCELED) {
			err = ErrCanceled
		}
		break
	case <-timerC:
		n, cqeFlags, err = vortex.cancelOperation(ctx, op)
		break
	case <-done:
		err = ctx.Err()
		if errors.Is(err, context.DeadlineExceeded) { // deadline so can cancel
			n, cqeFlags, err = vortex.cancelOperation(ctx, op)
		} else { // ctx canceled so try cancel and close op
			if op.canCancel() {
				vortex.Cancel(ctx, op)
			}
			op.Close()
			err = ErrCanceled
		}
		break
	}
	return
}

func (vortex *Vortex) cancelOperation(ctx context.Context, op *Operation) (n int, cqeFlags uint32, err error) {
	if vortex.Cancel(ctx, op) { // cancel succeed
		r, ok := <-op.resultCh
		if !ok {
			op.Close()
			err = ErrCanceled
			return
		}
		n, cqeFlags, err = r.N, r.Flags, r.Err
		if errors.Is(err, syscall.ECANCELED) {
			err = ErrTimeout
		}
		return
	}
	// cancel failed
	// means target op is not in ring, maybe completed or lost
	timer := vortex.acquireTimer(50 * time.Microsecond)
	defer vortex.releaseTimer(timer)

	select {
	case <-timer.C: // maybe lost
		op.Close()
		err = ErrCanceled
		break
	case r, ok := <-op.resultCh: // completed
		if !ok {
			err = ErrCanceled
			break
		}
		n, cqeFlags, err = r.N, r.Flags, r.Err
		if errors.Is(err, syscall.ECANCELED) {
			err = ErrTimeout
		}
		break
	}
	return
}
