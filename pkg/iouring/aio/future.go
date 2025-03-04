package aio

import (
	"context"
	"errors"
	"time"
	"unsafe"
)

type Future struct {
	vortex *Vortex
	op     *Operation
	err    error
}

func (f *Future) Await(ctx context.Context) (n int, err error) {
	op := f.op
	n, err = f.await(ctx)
	if op.borrowed {
		vortex := f.vortex
		vortex.releaseOperation(op)
	}
	return
}

func (f *Future) AwaitMsg(ctx context.Context) (n int, oobn int, flags int, addr unsafe.Pointer, addrLen uint32, err error) {
	op := f.op
	n, err = f.await(ctx)
	if err == nil {
		oobn = int(op.msg.Controllen)
		flags = int(op.msg.Flags)
		addr = unsafe.Pointer(op.msg.Name)
		addrLen = op.msg.Namelen
	}
	if op.borrowed {
		vortex := f.vortex
		vortex.releaseOperation(op)
	}
	return
}

const (
	ns500 = 500 * time.Nanosecond
)

func (f *Future) await(ctx context.Context) (n int, err error) {
	if f.err != nil {
		err = f.err
		return
	}
	vortex := f.vortex
	op := f.op
	timeout := op.Timeout(ctx)
	if timeout > 0 {
		timer := vortex.acquireTimer(timeout)
		select {
		case <-timer.C:
			if vortex.Cancel(op) {
				err = Timeout
				break
			}
			r := <-op.rch
			n, err = r.N, r.Err
			break
		case <-ctx.Done():
			if vortex.Cancel(op) {
				ctxErr := ctx.Err()
				if errors.Is(ctxErr, context.DeadlineExceeded) {
					err = Timeout
				} else {
					err = ctxErr
				}
				break
			}
			r := <-op.rch
			n, err = r.N, r.Err
			break
		case r := <-op.rch:
			n, err = r.N, r.Err
			break
		}
		vortex.releaseTimer(timer)
	} else {
		select {
		case <-ctx.Done():
			if vortex.Cancel(op) {
				ctxErr := ctx.Err()
				if errors.Is(ctxErr, context.DeadlineExceeded) {
					err = Timeout
				} else {
					err = ctxErr
				}
				return
			}
			r := <-op.rch
			n, err = r.N, r.Err
			break
		case r := <-op.rch:
			n, err = r.N, r.Err
			break
		}
	}
	return
}
