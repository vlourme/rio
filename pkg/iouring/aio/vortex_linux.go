//go:build linux

package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/kernel"
	"sync"
	"time"
)

func Open(ctx context.Context, options ...Option) (v *Vortex, err error) {
	// check kernel version
	version := kernel.Get()
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
		Entries:                  0,
		Flags:                    0,
		SQThreadCPU:              0,
		SQThreadIdle:             0,
		RegisterFixedBufferSize:  0,
		RegisterFixedBufferCount: 0,
		PrepSQEBatchSize:         0,
		PrepSQEBatchIdleTime:     0,
		PrepSQEBatchAffCPU:       -1,
		WaitCQEBatchSize:         0,
		WaitCQEBatchTimeCurve:    nil,
		WaitCQEBatchAffCPU:       -1,
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
		ring: ring,
	}
	return
}

type Vortex struct {
	operations sync.Pool
	timers     sync.Pool
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
	return
}

func (vortex *Vortex) submit(op *Operation) {
	vortex.ring.Submit(op)
	return
}

func (vortex *Vortex) Fd() int {
	return vortex.ring.Fd()
}

func (vortex *Vortex) AcquireBuffer() *FixedBuffer {
	return vortex.ring.AcquireBuffer()
}

func (vortex *Vortex) ReleaseBuffer(buf *FixedBuffer) {
	vortex.ring.ReleaseBuffer(buf)
}

func (vortex *Vortex) DirectAllocEnabled() bool {
	return vortex.ring.DirectAllocEnabled()
}

func (vortex *Vortex) RegisterFixedFdEnabled() bool {
	return vortex.ring.RegisterFixedFdEnabled()
}

func (vortex *Vortex) RegisterFixedFd(fd int) (index int, err error) {
	if vortex.RegisterFixedFdEnabled() {
		index, err = vortex.ring.RegisterFixedFd(fd)
		return
	}
	return -1, errors.New("register fixed fd is disabled")
}

func (vortex *Vortex) UnregisterFixedFd(index int) (err error) {
	return vortex.ring.UnregisterFixedFd(index)
}

func (vortex *Vortex) Shutdown() (err error) {
	err = vortex.ring.Close()
	return
}
