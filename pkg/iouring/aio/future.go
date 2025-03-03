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
	var deadline time.Time
	if timeout := op.Timeout(ctx); timeout > 0 {
		deadline = time.Now().Add(timeout)
	}

RETRY:
	r := op.getResult()
	if r == nil {
		// timeout
		if !deadline.IsZero() {
			if deadline.Before(time.Now()) {
				if vortex.Cancel(op) {
					err = Timeout
					return
				}
			}
		}
		// ctx
		if ctxErr := ctx.Err(); ctxErr != nil {
			if vortex.Cancel(op) {
				if errors.Is(ctxErr, context.DeadlineExceeded) {
					err = Timeout
				} else {
					err = ctxErr
				}
				return
			}
		}
		time.Sleep(ns500)
		goto RETRY
	}
	n, err = r.N, r.Err
	return
}
