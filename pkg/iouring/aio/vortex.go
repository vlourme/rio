//go:build linux

package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/kernel"
	"sync"
	"sync/atomic"
	"time"
)

func New(options ...Option) (v *Vortex, err error) {
	version := kernel.Get()
	if version.Invalidate() {
		err = errors.New("get kernel version failed")
		return
	}
	if !version.GTE(5, 19, 0) {
		err = errors.New("kernel version must greater than or equal to 5.19")
		return
	}

	opt := Options{
		Entries:                  0,
		Flags:                    0,
		SQThreadCPU:              0,
		SQThreadIdle:             0,
		RegisterFixedBufferSize:  0,
		RegisterFixedBufferCount: 0,
		PrepSQEBatchSize:         0,
		PrepSQEIdleTime:          0,
		PrepSQEAffCPU:            -1,
		WaitCQEBatchSize:         0,
		WaitCQETimeCurve:         nil,
		WaitCQEAffCPU:            -1,
	}
	for _, option := range options {
		option(&opt)
	}

	v = &Vortex{
		running: atomic.Bool{},
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
		options: opt,
		ring:    nil,
	}
	return
}

type Vortex struct {
	running    atomic.Bool
	operations sync.Pool
	timers     sync.Pool
	options    Options
	ring       IOURing
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
}

func (vortex *Vortex) submit(op *Operation) {
	vortex.ring.Submit(op)
}

func (vortex *Vortex) Cancel(target *Operation) (ok bool) {
	if target.canCancel() {
		op := &Operation{} // do not make ch cause no userdata
		op.PrepareCancel(target)
		vortex.submit(op)
		ok = true
		return
	}
	return
}

func (vortex *Vortex) AcquireBuffer() *FixedBuffer {
	return vortex.ring.AcquireBuffer()
}

func (vortex *Vortex) ReleaseBuffer(buf *FixedBuffer) {
	vortex.ring.ReleaseBuffer(buf)
}

func (vortex *Vortex) Shutdown() (err error) {
	if vortex.running.CompareAndSwap(true, false) {
		ring := vortex.ring
		vortex.ring = nil
		err = ring.Close()
	}
	return
}

func (vortex *Vortex) Start(ctx context.Context) (err error) {
	if !vortex.running.CompareAndSwap(false, true) {
		err = errors.New("vortex already running")
		return
	}

	options := vortex.options
	ring, ringErr := NewIOURing(options)
	if ringErr != nil {
		return ringErr
	}

	vortex.ring = ring

	ring.Start(ctx)

	return
}
