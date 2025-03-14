//go:build linux

package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"syscall"
	"time"
)

func (vortex *Vortex) Connect(ctx context.Context, fd int, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareConnect(fd, addr, addrLen)
	n, _, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) Accept(ctx context.Context, fd int, addr *syscall.RawSockaddrAny, addrLen int) (n int, err error) {
	op := vortex.acquireOperation()
	op.PrepareAccept(fd, addr, addrLen)
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

func (vortex *Vortex) ReadFixed(ctx context.Context, fd int, buf *FixedBuffer, deadline time.Time) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithRingId(buf.ringId).WithDeadline(deadline).PrepareReadFixed(fd, buf)
	n, _, err = vortex.submitAndWait(ctx, op)
	buf.rightShiftWritePosition(n)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) WriteFixed(ctx context.Context, fd int, buf *FixedBuffer, deadline time.Time) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithRingId(buf.ringId).WithDeadline(deadline).PrepareWriteFixed(fd, buf)
	n, _, err = vortex.submitAndWait(ctx, op)
	buf.rightShiftReadPosition(n)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) Receive(ctx context.Context, fd int, b []byte, deadline time.Time) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareReceive(fd, b)
	n, _, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) Send(ctx context.Context, fd int, b []byte, deadline time.Time) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSend(fd, b)
	n, _, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) SendZC(ctx context.Context, fd int, b []byte, deadline time.Time) (n int, err error) {
	op := vortex.acquireOperation()
	op.Hijack()
	op.WithDeadline(deadline).PrepareSendZC(fd, b)
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
		_, cqeFlags, err = vortex.AwaitOperation(ctx, op)
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

func (vortex *Vortex) ReceiveFrom(ctx context.Context, fd int, b []byte, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareReceiveMsg(fd, b, nil, addr, addrLen, 0)
	n, _, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) SendTo(ctx context.Context, fd int, b []byte, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSendMsg(fd, b, nil, addr, addrLen, 0)
	n, _, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) SendToZC(ctx context.Context, fd int, b []byte, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time) (n int, err error) {
	n, _, err = vortex.SendMsgZC(ctx, fd, b, nil, addr, addrLen, deadline)
	return
}

func (vortex *Vortex) ReceiveMsg(ctx context.Context, fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int, deadline time.Time) (n int, oobn int, flag int, err error) {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareReceiveMsg(fd, b, oob, addr, addrLen, int32(flags))
	n, _, err = vortex.submitAndWait(ctx, op)
	if err == nil {
		oobn = int(op.msg.Controllen)
		flag = int(op.msg.Flags)
	}
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) SendMsg(ctx context.Context, fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time) (n int, oobn int, err error) {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSendMsg(fd, b, oob, addr, addrLen, 0)
	n, _, err = vortex.submitAndWait(ctx, op)
	if err == nil {
		oobn = int(op.msg.Controllen)
	}
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) SendMsgZC(ctx context.Context, fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time) (n int, oobn int, err error) {
	op := vortex.acquireOperation()
	op.Hijack()
	op.WithDeadline(deadline).PrepareSendMsgZC(fd, b, oob, addr, addrLen, 0)
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
		_, cqeFlags, err = vortex.AwaitOperation(ctx, op)
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

func (vortex *Vortex) submitAndWait(ctx context.Context, op *Operation) (n int, cqeFlags uint32, err error) {
	deadline := op.deadline
RETRY:
	vortex.submit(op)
	n, cqeFlags, err = vortex.AwaitOperation(ctx, op)
	if err != nil {
		if errors.Is(err, syscall.EBUSY) {
			if !deadline.IsZero() && deadline.Before(time.Now()) {
				err = Timeout
				return
			}
			goto RETRY
		}
		return
	}
	return
}

func (vortex *Vortex) AwaitOperation(ctx context.Context, op *Operation) (n int, cqeFlags uint32, err error) {
	var (
		done                    = ctx.Done()
		timer  *time.Timer      = nil
		timerC <-chan time.Time = nil
	)
	timeout := op.Timeout()
	if timeout > 0 {
		timer = vortex.acquireTimer(timeout)
		timerC = timer.C
	} else if timeout < 0 {
		if vortex.Cancel(op) {
			err = Timeout
			return
		}
	}
	select {
	case r, ok := <-op.resultCh:
		if !ok {
			err = Uncompleted
			break
		}
		n, cqeFlags, err = r.N, r.Flags, r.Err
		break
	case <-done:
		if vortex.Cancel(op) {
			err = ctx.Err()
			if errors.Is(err, context.DeadlineExceeded) {
				err = Timeout
			}
			break
		}
		r, ok := <-op.resultCh
		if !ok {
			err = Uncompleted
			break
		}
		n, cqeFlags, err = r.N, r.Flags, r.Err
		break
	case <-timerC:
		if vortex.Cancel(op) {
			err = Timeout
			break
		}
		r, ok := <-op.resultCh
		if !ok {
			err = Uncompleted
			break
		}
		n, cqeFlags, err = r.N, r.Flags, r.Err
		break
	}
	if timer != nil {
		vortex.releaseTimer(timer)
	}
	return
}
