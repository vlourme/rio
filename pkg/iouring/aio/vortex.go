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

var (
	Uncompleted   = errors.New("uncompleted")
	Timeout       = &TimeoutError{}
	UnsupportedOp = errors.New("unsupported op")
)

type TimeoutError struct{}

func (e *TimeoutError) Error() string   { return "i/o timeout" }
func (e *TimeoutError) Timeout() bool   { return true }
func (e *TimeoutError) Temporary() bool { return true }

func (e *TimeoutError) Is(err error) bool {
	return err == context.DeadlineExceeded
}

func IsUncompleted(err error) bool {
	return errors.Is(err, Uncompleted)
}

func IsTimeout(err error) bool {
	return errors.Is(err, Timeout) || errors.Is(err, context.DeadlineExceeded)
}

func IsUnsupported(err error) bool {
	return errors.Is(err, UnsupportedOp)
}

type VortexOptions struct {
	Entries          uint32
	Flags            uint32
	Features         uint32
	PrepareBatchSize uint32
	UseCPUAffinity   bool
	WaitTransmission Transmission
}

func (options *VortexOptions) prepare() {
	if options.Entries == 0 {
		options.Entries = iouring.DefaultEntries
	}
	if options.Flags == 0 && options.Features == 0 {
		options.Flags, options.Features = DefaultIOURingFlagsAndFeatures()
	}
	if options.WaitTransmission == nil {
		options.WaitTransmission = NewCurveTransmission(defaultCurve)
	}
}

const (
	minKernelVersionMajor = 5
	minKernelVersionMinor = 1
)

func NewVortex(options VortexOptions) (v *Vortex) {
	ver, verErr := kernel.Get()
	if verErr != nil {
		panic(errors.New("get kernel version failed"))
		return nil
	}
	target := kernel.Version{
		Kernel: ver.Kernel,
		Major:  minKernelVersionMajor,
		Minor:  minKernelVersionMinor,
		Flavor: ver.Flavor,
	}

	if kernel.Compare(*ver, target) < 0 {
		panic(errors.New("kernel version too low"))
		return nil
	}

	options.prepare()

	// vortex
	v = &Vortex{
		ring:    nil,
		queue:   NewQueue[Operation](),
		options: options,
		operations: sync.Pool{
			New: func() interface{} {
				return &Operation{
					kind:     iouring.OpLast,
					borrowed: true,
					result:   atomic.Pointer[Result]{},
				}
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
	wg         sync.WaitGroup
	options    VortexOptions
	ring       *iouring.Ring
	queue      *Queue[Operation]
}

func (vortex *Vortex) SetId(id int) {
	if id < 1 {
		id = 0
	}
	vortex.id = id
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

func (vortex *Vortex) Close() (err error) {
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
	uring, uringErr := iouring.New(options.Entries, options.Flags, options.Features, nil)
	if uringErr != nil {
		return uringErr
	}
	vortex.ring = uring

	vortex.wg.Add(1)
	go func(ctx context.Context, vortex *Vortex) {
		// cpu affinity
		threadLocked := false
		if vortex.options.UseCPUAffinity {
			if setErr := process.SetCPUAffinity(vortex.id); setErr == nil {
				threadLocked = true
				runtime.LockOSThread()
			}
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
				time.Sleep(ns500)
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
