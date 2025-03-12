//go:build linux

package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/kernel"
	"github.com/brickingsoft/rio/pkg/semaphores"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func New(options ...Option) (v *Vortex, err error) {
	version := kernel.Get()
	if version.Invalidate() {
		err = errors.New("get kernel version failed")
		return
	}
	if !version.GTE(5, 1, 0) {
		err = errors.New("kernel version must greater than or equal to 5.1")
		return
	}

	opt := Options{}
	for _, option := range options {
		option(&opt)
	}
	// sq semaphores
	prepareIdleTime := opt.PrepSQEIdleTime
	if prepareIdleTime < 1 {
		prepareIdleTime = defaultPrepareSQIdleTime
	}
	sqSemaphores, sqSemaphoresErr := semaphores.New(prepareIdleTime)
	if sqSemaphoresErr != nil {
		err = sqSemaphoresErr
		return
	}

	v = &Vortex{
		ring:    nil,
		queue:   NewQueue[Operation](),
		options: opt,
		operations: sync.Pool{
			New: func() interface{} {
				return &Operation{
					kind:     iouring.OpLast,
					borrowed: true,
					resultCh: make(chan Result),
					ringId:   -1,
				}
			},
		},
		timers: sync.Pool{
			New: func() interface{} {
				return time.NewTimer(0)
			},
		},
		running:      atomic.Bool{},
		wg:           sync.WaitGroup{},
		sqSemaphores: sqSemaphores,
		buffers:      NewQueue[FixedBuffer](),
	}
	return
}

type Vortex struct {
	ctx        context.Context
	cancel     context.CancelFunc
	running    atomic.Bool
	operations sync.Pool
	timers     sync.Pool
	options    Options
	rings      []*URing

	wg           sync.WaitGroup
	sqSemaphores *semaphores.Semaphores
	ring         *iouring.Ring
	queue        *Queue[Operation]
	buffers      *Queue[FixedBuffer]
}

func (vortex *Vortex) acquireOperation() *Operation {
	op := vortex.operations.Get().(*Operation)
	return op
}

func (vortex *Vortex) releaseOperation(op *Operation) {
	if op.borrowed {
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
	vortex.queue.Enqueue(op)
	vortex.sqSemaphores.Signal()
}

func (vortex *Vortex) Cancel(target *Operation) (ok bool) {
	if target.status.CompareAndSwap(ReadyOperationStatus, CompletedOperationStatus) || target.status.CompareAndSwap(ProcessingOperationStatus, CompletedOperationStatus) {
		op := &Operation{} // do not make ch cause no userdata
		op.PrepareCancel(target)
		vortex.submit(op)
		ok = true
		return
	}
	return
}

func (vortex *Vortex) AcquireBuffer() *FixedBuffer {
	if vortex.options.RegisterFixedBufferSize == 0 {
		return nil
	}
	return vortex.buffers.Dequeue()
}

func (vortex *Vortex) ReleaseBuffer(buf *FixedBuffer) {
	if vortex.options.RegisterFixedBufferSize == 0 || buf == nil {
		return
	}
	buf.Reset()
	vortex.buffers.Enqueue(buf)
}

func (vortex *Vortex) Done() <-chan struct{} {
	if ctx := vortex.ctx; ctx != nil {
		return ctx.Done()
	}
	ch := make(chan struct{})
	close(ch)
	return ch
}

func (vortex *Vortex) Shutdown() (err error) {
	if vortex.running.CompareAndSwap(true, false) {
		vortex.cancel()
		vortex.cancel = nil
		vortex.wg.Wait()
		ring := vortex.ring
		vortex.ring = nil
		if vortex.options.RegisterFixedBufferCount > 0 {
			_, _ = ring.UnregisterBuffers()
			for {
				if buf := vortex.buffers.Dequeue(); buf == nil {
					break
				}
			}
		}
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
	uring, uringErr := iouring.New(
		iouring.WithEntries(options.Entries),
		iouring.WithFlags(options.Flags),
		iouring.WithSQThreadIdle(options.SQThreadIdle),
		iouring.WithSQThreadCPU(options.SQThreadCPU),
	)
	if uringErr != nil {
		return uringErr
	}

	// register buffers
	if size, count := vortex.options.RegisterFixedBufferSize, vortex.options.RegisterFixedBufferCount; count > 0 && size > 0 {
		iovecs := make([]syscall.Iovec, count)
		for i := uint32(0); i < count; i++ {
			buf := make([]byte, size)
			vortex.buffers.Enqueue(&FixedBuffer{
				value: buf,
				index: int(i),
			})
			iovecs[i] = syscall.Iovec{
				Base: &buf[0],
				Len:  uint64(size),
			}
		}
		_, regErr := uring.RegisterBuffers(iovecs)
		if regErr != nil {
			vortex.options.RegisterFixedBufferSize, vortex.options.RegisterFixedBufferCount = 0, 0
			for i := uint32(0); i < count; i++ {
				_ = vortex.buffers.Dequeue()
			}
		}
	}

	vortex.ring = uring

	cc, cancel := context.WithCancel(ctx)
	vortex.ctx = cc
	vortex.cancel = cancel

	vortex.wg.Add(2)
	go vortex.preparingSQE(cc)
	go vortex.waitingCQE(cc)

	return
}

func (vortex *Vortex) preparingSQE(ctx context.Context) {
	defer vortex.wg.Done()

	// lock os thread
	//runtime.LockOSThread()
	//defer runtime.UnlockOSThread()

	// cpu affinity
	//_ = process.SetCPUAffinity(int(vortex.options.PrepareSQEAffinityCPU))

	ring := vortex.ring

	queue := vortex.queue
	shs := vortex.sqSemaphores
	var (
		peeked       uint32
		prepared     uint32
		needToSubmit int64
	)

	prepareBatch := vortex.options.PrepSQEBatchSize
	if prepareBatch < 1 {
		prepareBatch = 1024
	}
	operations := make([]*Operation, prepareBatch)
	for {
		if peeked = queue.PeekBatch(operations); peeked == 0 {
			if needToSubmit > 0 {
				submitted, _ := ring.Submit()
				needToSubmit -= int64(submitted)
			}
			if waitErr := shs.Wait(ctx); waitErr != nil {
				if errors.Is(waitErr, context.Canceled) {
					break
				}
			}
			continue
		}
		for i := uint32(0); i < peeked; i++ {
			op := operations[i]
			if op == nil {
				continue
			}
			operations[i] = nil
			if op.status.CompareAndSwap(ReadyOperationStatus, ProcessingOperationStatus) {
				if prepErr := vortex.prepareSQE(op); prepErr != nil { // when prep err occur, means invalid op kind or no sqe left
					op.setResult(0, prepErr)
					if errors.Is(prepErr, syscall.EBUSY) { // no sqe left
						break
					}
					prepared++ // prepareSQE nop whit out userdata, so prepared++
					continue
				}
				prepared++
			}
		}
		// submit
		if prepared > 0 || needToSubmit > 0 {
			submitted, _ := ring.Submit()
			needToSubmit += int64(prepared) - int64(submitted)
			vortex.queue.Advance(prepared)
			prepared = 0
		}
	}
	// evict
	if remains := vortex.queue.Length(); remains > 0 {
		peeked = vortex.queue.PeekBatch(operations)
		for i := uint32(0); i < peeked; i++ {
			op := operations[i]
			operations[i] = nil
			op.setResult(0, Uncompleted)
		}
	}
	// prepare noop to wakeup cq waiter
	for {
		sqe := ring.GetSQE()
		if sqe == nil {
			time.Sleep(1 * time.Millisecond)
			continue
		}
		sqe.PrepareNop()
		for {
			if _, subErr := ring.Submit(); subErr != nil {
				time.Sleep(1 * time.Millisecond)
				continue
			}
			break
		}
		break
	}
	return
}

func (vortex *Vortex) waitingCQE(ctx context.Context) {
	defer vortex.wg.Done()

	// lock os thread
	//runtime.LockOSThread()
	//defer runtime.UnlockOSThread()

	// cpu affinity
	//_ = process.SetCPUAffinity(int(vortex.options.WaitCQEAffinityCPU))

	ring := vortex.ring
	transmission := NewCurveTransmission(vortex.options.WaitCQETimeCurve)
	cqeWaitMaxCount, cqeWaitTimeout := transmission.Up()
	waitCQBatchSize := vortex.options.WaitCQEBatchSize
	if waitCQBatchSize < 1 {
		waitCQBatchSize = 1024
	}
	cq := make([]*iouring.CompletionQueueEvent, waitCQBatchSize)
	stopped := false
	for {
		select {
		case <-ctx.Done():
			stopped = true
			break
		default:
			if completed := ring.PeekBatchCQE(cq); completed > 0 {
				for i := uint32(0); i < completed; i++ {
					cqe := cq[i]
					cq[i] = nil
					if cqe.UserData == 0 { // no userdata means no op
						continue
					}
					if notify := cqe.Flags&iouring.CQEFNotify != 0; notify { // used by send zc
						continue
					}
					// get op from
					copPtr := cqe.GetData()
					cop := (*Operation)(copPtr)
					// handle
					if cop.status.CompareAndSwap(ProcessingOperationStatus, CompletedOperationStatus) { // not done
						var (
							opN   int
							opErr error
						)
						if cqe.Res < 0 {
							opErr = os.NewSyscallError(cop.Name(), syscall.Errno(-cqe.Res))
						} else {
							opN = int(cqe.Res)
						}
						cop.setResult(opN, opErr)
					}
				}
				// CQAdvance
				ring.CQAdvance(completed)
			} else {
				if _, waitErr := ring.WaitCQEs(cqeWaitMaxCount, cqeWaitTimeout, nil); waitErr != nil {
					cqeWaitMaxCount, cqeWaitTimeout = transmission.Down()
				} else {
					cqeWaitMaxCount, cqeWaitTimeout = transmission.Up()
				}
			}
			break
		}

		if stopped {
			break
		}
	}

	return
}
