package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/kernel"
	"github.com/brickingsoft/rio/pkg/process"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func New(options ...Option) (v *Vortex, err error) {
	ver, verErr := kernel.Get()
	if verErr != nil {
		err = errors.New("get kernel version failed")
		return
	}

	target := kernel.Version{
		Kernel: ver.Kernel,
		Major:  minKernelVersionMajor,
		Minor:  minKernelVersionMinor,
		Flavor: ver.Flavor,
	}

	if kernel.Compare(*ver, target) < 0 {
		err = errors.New("kernel version too low")
		return
	}

	opt := Options{
		Entries:          0,
		Flags:            DefaultIOURingSetupSchema(),
		SQThreadCPU:      0,
		SQThreadIdle:     0,
		PrepareBatchSize: 0,
		UseCPUAffinity:   false,
		WaitTransmission: nil,
	}
	for _, option := range options {
		option(&opt)
	}
	// default flags and feats
	if opt.Flags&iouring.SetupSQPoll != 0 && opt.SQThreadIdle == 0 {
		opt.SQThreadIdle = 500 // default sq thread idle is 500 milli
	}
	// default wait transmission
	if opt.WaitTransmission == nil {
		opt.WaitTransmission = NewCurveTransmission(defaultCurve)
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
					rch:      make(chan Result),
				}
			},
		},
		timers: sync.Pool{
			New: func() interface{} {
				return time.NewTimer(0)
			},
		},
		running: atomic.Bool{},
		stopped: atomic.Bool{},
		wg:      sync.WaitGroup{},
	}
	return
}

type Vortex struct {
	id         int
	running    atomic.Bool
	stopped    atomic.Bool
	operations sync.Pool
	timers     sync.Pool
	wg         sync.WaitGroup
	options    Options
	ring       *iouring.Ring
	queue      *Queue[Operation]
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
}

func (vortex *Vortex) Cancel(target *Operation) (ok bool) {
	if target.status.CompareAndSwap(ReadyOperationStatus, CompletedOperationStatus) || target.status.CompareAndSwap(ProcessingOperationStatus, CompletedOperationStatus) {
		op := &Operation{} // do not make ch cause no userdata
		op.PrepareCancel(target)
		vortex.queue.Enqueue(op)
		ok = true
		return
	}
	return
}

func (vortex *Vortex) Shutdown() (err error) {
	if vortex.running.CompareAndSwap(true, false) {
		vortex.stopped.Store(true)
		vortex.wg.Wait()
		ring := vortex.ring
		vortex.ring = nil
		err = ring.Close()
	}
	return
}

const (
	defaultWaitTimeout = 1 * time.Microsecond
)

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
	vortex.ring = uring

	vortex.wg.Add(1)
	go func(ctx context.Context, vortex *Vortex) {
		// cpu affinity
		threadLocked := false
		if vortex.options.UseCPUAffinity || vortex.options.Flags&iouring.SetupSingleIssuer != 0 {
			_ = process.SetCPUAffinity(vortex.id)
			threadLocked = true
			runtime.LockOSThread()
		}

		ring := vortex.ring

		prepareBatch := vortex.options.PrepareBatchSize
		if prepareBatch < 1 {
			prepareBatch = ring.SQEntries()
		}
		operations := make([]*Operation, prepareBatch)

		waitTransmission := vortex.options.WaitTransmission
		cq := make([]*iouring.CompletionQueueEvent, ring.CQEntries())

		var (
			processing uint32
			submittedN uint32
		)
		for {
			if ctxErr := ctx.Err(); ctxErr != nil {
				break
			}
			if vortex.stopped.Load() {
				break
			}
			// handle sqe
			if peeked := vortex.queue.PeekBatch(operations); peeked > 0 {
				// peek
				prepared := uint32(0)
				for i := uint32(0); i < peeked; i++ {
					op := operations[i]
					if op == nil {
						break
					}
					operations[i] = nil
					if op.status.CompareAndSwap(ReadyOperationStatus, ProcessingOperationStatus) {
						if prepErr := vortex.prepareSQE(op); prepErr != nil {
							if prepErr != nil { // when prep err occur, means invalid op kind,
								op.setResult(0, prepErr)
								if errors.Is(prepErr, syscall.EBUSY) { // no sqe left
									break
								}
								prepared++ // prepareSQE nop whit out userdata, so prepared++
								continue
							}
						}
						runtime.KeepAlive(op)
						prepared++
					} else { // maybe canceled
						vortex.releaseOperation(op)
					}
				}
				// submit
				for {
					submitted, submitErr := ring.Submit()
					if submitErr != nil {
						if ctxErr := ctx.Err(); ctxErr != nil {
							break
						}
						if vortex.stopped.Load() {
							break
						}
					}
					submittedN += uint32(submitted)
					if submittedN >= prepared {
						vortex.queue.Advance(prepared)
						processing += prepared
						break
					}
				}
				submittedN = 0
			}
			// handle cqe
			if processing > 0 {
				// wait
				waitTimeout := waitTransmission.Match(processing)
				if waitTimeout.Sec == 0 && waitTimeout.Nsec == 0 {
					waitTimeout = syscall.NsecToTimespec(defaultWaitTimeout.Nanoseconds())
				}
				_, _ = ring.WaitCQEs(processing, &waitTimeout, nil)

				// peek cqe
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
						} else { // done
							// canceled then release op
							vortex.releaseOperation(cop)
						}
					}
					// CQAdvance
					ring.CQAdvance(completed)
					processing -= completed
				}
			} else {
				time.Sleep(defaultWaitTimeout)
			}
		}

		// evict remain
		if remains := vortex.queue.Length(); remains > 0 {
			peeked := vortex.queue.PeekBatch(operations)
			for i := uint32(0); i < peeked; i++ {
				op := operations[i]
				operations[i] = nil
				op.setResult(0, Uncompleted)
			}
		}

		// unlock os thread
		if threadLocked {
			runtime.UnlockOSThread()
		}
		// done
		vortex.wg.Done()
	}(ctx, vortex)
	return nil
}
