package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"syscall"
)

type Future struct {
	vortex   *Vortex
	op       *Operation
	acquired bool
	err      error
}

func (f *Future) Await(ctx context.Context) (n int, err error) {
	hijacked := false
	n, hijacked, err = f.await(ctx)
	if f.acquired {
		vortex := f.vortex
		op := f.op
		if hijacked {
			vortex.hijackedOps.Store(op, struct{}{})
		} else {
			vortex.releaseOperation(op)
		}
	}
	return
}

func (f *Future) AwaitMsg(ctx context.Context) (n int, msg syscall.Msghdr, err error) {
	op := f.op
	hijacked := false
	n, hijacked, err = f.await(ctx)
	if err == nil {
		msg = op.msg
	}
	if f.acquired {
		vortex := f.vortex
		if hijacked {
			vortex.hijackedOps.Store(op, struct{}{})
		} else {
			vortex.releaseOperation(op)
		}
	}
	return
}

func (f *Future) await(ctx context.Context) (n int, hijacked bool, err error) {
	if f.err != nil {
		err = f.err
		return
	}
	vortex := f.vortex
	op := f.op
	ch := op.ch

	timeout := op.Timeout(ctx)
	switch {
	case timeout > 0: // await with timeout
		timer := vortex.acquireTimer(timeout)
		select {
		case r := <-ch:
			n, err = r.N, r.Err
			hijacked = r.Flags&iouring.CQEFMore != 0
			break
		case <-timer.C:
			if vortex.Cancel(op) {
				err = Timeout
				break
			}
			// op has been completed, so continue to fetch result
			r := <-ch
			n, err = r.N, r.Err
			hijacked = r.Flags&iouring.CQEFMore != 0
			break
		case <-ctx.Done():
			if vortex.Cancel(op) {
				err = ctx.Err()
				if errors.Is(err, context.DeadlineExceeded) {
					err = Timeout
				}
				break
			}
			// op has been completed, so continue to fetch result
			r := <-ch
			n, err = r.N, r.Err
			hijacked = r.Flags&iouring.CQEFMore != 0
			break
		}
		vortex.releaseTimer(timer)
		break
	case timeout < 0: // timeout then try cancel
		if vortex.Cancel(op) {
			err = Timeout
		} else {
			// op has been completed, so continue to fetch result
			r := <-ch
			n, err = r.N, r.Err
			hijacked = r.Flags&iouring.CQEFMore != 0
		}
		break
	default: // await without timeout
		select {
		case r := <-ch:
			n, err = r.N, r.Err
			hijacked = r.Flags&iouring.CQEFMore != 0
			break
		case <-ctx.Done():
			if vortex.Cancel(op) {
				err = ctx.Err()
				if errors.Is(err, context.DeadlineExceeded) {
					err = Timeout
				}
				break
			}
			// op has been completed, so continue to fetch result
			r := <-ch
			n, err = r.N, r.Err
			hijacked = r.Flags&iouring.CQEFMore != 0
			break
		}
		break
	}
	return
}
