//go:build linux

package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/kernel"
	"github.com/brickingsoft/rio/pkg/process"
	"github.com/brickingsoft/rio/pkg/semaphores"
	"os"
	"runtime"
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
	if !version.GTE(5, 19, 0) {
		err = errors.New("kernel version must greater than or equal to 5.19")
		return
	}

	opt := Options{}
	for _, option := range options {
		option(&opt)
	}
	// op semaphores
	prepareIdleTime := opt.PrepSQEIdleTime
	if prepareIdleTime < 1 {
		prepareIdleTime = defaultPrepareSQIdleTime
	}
	submitSemaphores, submitSemaphoresErr := semaphores.New(prepareIdleTime)
	if submitSemaphoresErr != nil {
		err = submitSemaphoresErr
		return
	}

	v = &Vortex{
		ring:     nil,
		requests: NewQueue[Operation](),
		options:  opt,
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
		running:          atomic.Bool{},
		wg:               sync.WaitGroup{},
		submitSemaphores: submitSemaphores,
		buffers:          NewQueue[FixedBuffer](),
	}
	return
}

type Vortex struct {
	running          atomic.Bool
	operations       sync.Pool
	timers           sync.Pool
	options          Options
	wg               sync.WaitGroup
	submitSemaphores *semaphores.Semaphores
	ring             *iouring.Ring
	requests         *Queue[Operation]
	buffers          *Queue[FixedBuffer]
	cancel           context.CancelFunc
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
	vortex.requests.Enqueue(op)
	vortex.submitSemaphores.Signal()
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
	vortex.cancel = cancel

	vortex.wg.Add(2)
	go vortex.preparingSQE(cc)
	go vortex.waitingCQE(cc)

	return
}

func (vortex *Vortex) preparingSQE(ctx context.Context) {
	defer vortex.wg.Done()

	// cpu affinity
	if vortex.ring.Flags()&iouring.SetupSingleIssuer != 0 && vortex.options.PrepSQEAffCPU == -1 {
		vortex.options.PrepSQEAffCPU = 0
		if vortex.ring.Flags()&iouring.SetupSQAff != 0 && uint32(vortex.options.PrepSQEAffCPU) == vortex.options.SQThreadCPU {
			vortex.options.PrepSQEAffCPU++
		}
	}
	if cpu := vortex.options.PrepSQEAffCPU; cpu > -1 {
		runtime.LockOSThread()
		if setErr := process.SetCPUAffinity(cpu); setErr != nil {
			runtime.UnlockOSThread()
		} else {
			defer runtime.UnlockOSThread()
		}
	}

	ring := vortex.ring

	queue := vortex.requests
	shs := vortex.submitSemaphores
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
			if op.canPrepare() {
				if prepErr := vortex.prepareSQE(op); prepErr != nil { // when prep err occur, means invalid op kind or no sqe left
					op.setResult(0, 0, prepErr)
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
			vortex.requests.Advance(prepared)
			prepared = 0
		}
	}
	// evict
	if remains := vortex.requests.Length(); remains > 0 {
		peeked = vortex.requests.PeekBatch(operations)
		for i := uint32(0); i < peeked; i++ {
			op := operations[i]
			operations[i] = nil
			op.setResult(0, 0, Uncompleted)
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

	// cpu affinity
	if cpu := vortex.options.WaitCQEAffCPU; cpu > -1 {
		runtime.LockOSThread()
		if setErr := process.SetCPUAffinity(cpu); setErr != nil {
			runtime.UnlockOSThread()
		} else {
			defer runtime.UnlockOSThread()
		}
	}

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
					if cqe.IsInternalUpdateTimeoutUserdata() { // userdata means not op
						continue
					}

					// get op from cqe
					copPtr := cqe.GetData()
					cop := (*Operation)(copPtr)

					// handle
					if cop.canSetResult() { // not done
						var (
							opN     int
							opFlags = cqe.Flags
							opErr   error
						)
						if cqe.Res < 0 {
							opErr = os.NewSyscallError(cop.Name(), syscall.Errno(-cqe.Res))
						} else {
							opN = int(cqe.Res)
						}
						cop.setResult(opN, opFlags, opErr)
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
