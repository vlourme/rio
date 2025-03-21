//go:build linux

package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"sync"
	"syscall"
	"time"
)

func Open(ctx context.Context, options ...Option) (v *Vortex, err error) {
	// check kernel version
	version := iouring.GetVersion()
	if version.Invalidate() {
		err = errors.New("get kernel version failed")
		return
	}
	if !version.GTE(6, 0, 0) {
		err = errors.New("kernel version must greater than or equal to 6.0")
		return
	}
	// options
	opt := Options{
		PrepSQEAffCPU: -1,
		WaitCQEMode:   WaitCQEPushMode,
	}
	for _, option := range options {
		option(&opt)
	}
	// ring
	ring, ringErr := OpenIOURing(ctx, opt)
	if ringErr != nil {
		err = ringErr
		return
	}
	// vortex
	v = &Vortex{
		IOURing: ring,
		operations: sync.Pool{
			New: func() interface{} {
				return &Operation{
					kind:     iouring.OpLast,
					borrowed: true,
					resultCh: make(chan Result, 1),
				}
			},
		},
		timers: sync.Pool{
			New: func() interface{} {
				return time.NewTimer(0)
			},
		},
	}
	return
}

type Vortex struct {
	IOURing
	operations sync.Pool
	timers     sync.Pool
}

func (vortex *Vortex) CancelOperation(ctx context.Context, target *Operation) (ok bool) {
	op := vortex.acquireOperation()
	op.PrepareCancel(target)
	_, _, err := vortex.submitAndWait(ctx, op)
	ok = err == nil
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) FixedFdInstall(ctx context.Context, directFd int) (regularFd int, err error) {
	op := vortex.acquireOperation()
	op.PrepareFixedFdInstall(directFd)
	regularFd, _, err = vortex.submitAndWait(ctx, op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) acquireOperation() *Operation {
	op := vortex.operations.Get().(*Operation)
	return op
}

func (vortex *Vortex) releaseOperation(op *Operation) {
	if op.canRelease() {
		op.reset()
		vortex.operations.Put(op)
	}
}

func (vortex *Vortex) acquireTimer(timeout time.Duration) *time.Timer {
	timer := vortex.timers.Get().(*time.Timer)
	timer.Reset(timeout)
	return timer
}

func (vortex *Vortex) releaseTimer(timer *time.Timer) {
	timer.Stop()
	vortex.timers.Put(timer)
	return
}

func (vortex *Vortex) submitAndWait(ctx context.Context, op *Operation) (n int, cqeFlags uint32, err error) {
	deadline := op.deadline
RETRY:
	vortex.Submit(op)
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
				vortex.CancelOperation(ctx, op)
			}
			op.Close()
			err = ErrCanceled
		}
		break
	}
	return
}

func (vortex *Vortex) cancelOperation(ctx context.Context, op *Operation) (n int, cqeFlags uint32, err error) {
	if vortex.CancelOperation(ctx, op) { // cancel succeed
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
