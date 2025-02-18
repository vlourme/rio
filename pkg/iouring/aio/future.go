package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
)

type Future struct {
	vortex   *Vortex
	op       *Operation
	acquired bool
	err      error
}

func (f *Future) Await(ctx context.Context) (n int, err error) {
	if f.err != nil {
		err = f.err
		return
	}
	vortex := f.vortex
	op := f.op
	ch := op.ch
	hijacked := false
	if timeout := op.timeout; timeout > 0 {
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
	} else {
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
	}
	if f.acquired {
		if hijacked {
			vortex.hijackedOps.Store(op, struct{}{})
		} else {
			vortex.releaseOperation(op)
		}
	}
	return
}
