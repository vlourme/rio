//go:build linux

package aio

import (
	"context"
	"errors"
	"syscall"
	"time"
)

func (vortex *Vortex) Connect(ctx context.Context, fd int, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareConnect(fd, addr, addrLen)
	n, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) Accept(ctx context.Context, fd int, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareAccept(fd, addr, addrLen)
	n, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) Close(ctx context.Context, fd int) (err error) {
	op := vortex.acquireOperation()
	op.PrepareClose(fd)
	_, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) ReadFixed(ctx context.Context, fd int, buf *FixedBuffer, deadline time.Time) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareReadFixed(fd, buf)
	n, err = vortex.submitAndWait(ctx, op)
	buf.rightShiftWritePosition(n)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) WriteFixed(ctx context.Context, fd int, buf *FixedBuffer, deadline time.Time) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareWriteFixed(fd, buf)
	n, err = vortex.submitAndWait(ctx, op)
	buf.rightShiftReadPosition(n)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) Receive(ctx context.Context, fd int, b []byte, deadline time.Time) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareReceive(fd, b)
	n, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) Send(ctx context.Context, fd int, b []byte, deadline time.Time) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSend(fd, b)
	n, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) SendZC(ctx context.Context, fd int, b []byte, deadline time.Time) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSendZC(fd, b)
	n, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) ReceiveFrom(ctx context.Context, fd int, b []byte, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareReceiveMsg(fd, b, nil, addr, addrLen, 0)
	n, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) SendTo(ctx context.Context, fd int, b []byte, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSendMsg(fd, b, nil, addr, addrLen, 0)
	n, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) SendToZC(ctx context.Context, fd int, b []byte, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time) (n int, err error) {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSendMsgZC(fd, b, nil, addr, addrLen, 0)
	n, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) ReceiveMsg(ctx context.Context, fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int, deadline time.Time) (n int, oobn int, flag int, err error) {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareReceiveMsg(fd, b, oob, addr, addrLen, int32(flags))
	n, err = vortex.submitAndWait(ctx, op)
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
	n, err = vortex.submitAndWait(ctx, op)
	if err == nil {
		oobn = int(op.msg.Controllen)
	}
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) SendMsgZC(ctx context.Context, fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time) (n int, oobn int, err error) {
	op := vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareSendMsgZC(fd, b, oob, addr, addrLen, 0)
	n, err = vortex.submitAndWait(ctx, op)
	if err == nil {
		oobn = int(op.msg.Controllen)
	}
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) submitAndWait(ctx context.Context, op *Operation) (n int, err error) {
	deadline := op.deadline
RETRY:
	vortex.submit(op)
	n, err = vortex.await(ctx, op)
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

func (vortex *Vortex) await(ctx context.Context, op *Operation) (n int, err error) {
	// todo: add case vortex.Done()
	timeout := op.Timeout()
	switch {
	case timeout == 0:
		select {
		case r, ok := <-op.resultCh:
			if !ok {
				err = Uncompleted
				break
			}
			n, err = r.N, r.Err
			break
		case <-ctx.Done():
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
			n, err = r.N, r.Err
			break
		}
		break
	case timeout > 0:
		timer := vortex.acquireTimer(timeout)
		select {
		case r, ok := <-op.resultCh:
			if !ok {
				err = Uncompleted
				break
			}
			n, err = r.N, r.Err
			break
		case <-timer.C:
			if vortex.Cancel(op) {
				err = Timeout
				break
			}
			r, ok := <-op.resultCh
			if !ok {
				err = Uncompleted
				break
			}
			n, err = r.N, r.Err
			break
		case <-ctx.Done():
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
			n, err = r.N, r.Err
			break
		}
		vortex.releaseTimer(timer)
		break
	case timeout < 0:
		if vortex.Cancel(op) {
			err = Timeout
			break
		}
		r, ok := <-op.resultCh
		if !ok {
			err = Uncompleted
			break
		}
		n, err = r.N, r.Err
		break
	}
	return
}
