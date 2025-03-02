package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/kernel"
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

func NewVortex(options VortexOptions) (v *Vortex, err error) {
	ver, verErr := kernel.Get()
	if verErr != nil {
		return nil, verErr
	}
	target := kernel.Version{
		Kernel: ver.Kernel,
		Major:  minKernelVersionMajor,
		Minor:  minKernelVersionMinor,
		Flavor: ver.Flavor,
	}

	if kernel.Compare(*ver, target) < 0 {
		return nil, errors.New("kernel version too low")
	}

	options.prepare()
	// iouring
	ring, ringErr := iouring.New(options.Entries, options.Flags, options.Features, nil)
	if ringErr != nil {
		return nil, ringErr
	}
	// vortex
	v = &Vortex{
		ring:             ring,
		ops:              NewQueue[Operation](),
		prepareBatch:     options.PrepareBatchSize,
		lockOSThread:     options.Flags&iouring.SetupSingleIssuer != 0 || runtime.NumCPU() > 1,
		waitTransmission: options.WaitTransmission,
		operations: sync.Pool{
			New: func() interface{} {
				return &Operation{
					kind:     iouring.OpLast,
					borrowed: true,
					result:   NewQueue[Result](),
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
	ring             *iouring.Ring
	ops              *Queue[Operation]
	lockOSThread     bool
	prepareBatch     uint32
	waitTransmission Transmission
	operations       sync.Pool
	running          atomic.Bool
	stopped          atomic.Bool
	wg               sync.WaitGroup
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
		vortex.ops.Enqueue(op)
		ok = true
		return
	}
	return
}

func (vortex *Vortex) Close() (err error) {
	if vortex.running.CompareAndSwap(true, false) {
		vortex.stopped.Store(true)
		vortex.wg.Wait()
		err = vortex.ring.Close()
		return
	}
	if !vortex.stopped.Load() {
		err = vortex.ring.Close()
	}
	return
}

const (
	defaultWaitCQENr   = uint32(1)
	defaultWaitTimeout = 1 * time.Microsecond
)

func (vortex *Vortex) Start(ctx context.Context) {
	vortex.running.Store(true)
	vortex.wg.Add(1)
	go func(ctx context.Context, vortex *Vortex) {
		// lock os thread
		if vortex.lockOSThread {
			runtime.LockOSThread()
		}

		ring := vortex.ring

		prepareBatch := vortex.prepareBatch
		if prepareBatch < 1 {
			prepareBatch = ring.SQEntries()
		}
		operations := make([]*Operation, prepareBatch)

		waitTransmission := vortex.waitTransmission
		waitCQENr, waitCQETimeout := waitTransmission.MatchN(1)
		if waitCQENr < 1 {
			waitCQENr = defaultWaitCQENr
		}
		if waitCQETimeout < 1 {
			waitCQETimeout = defaultWaitTimeout
		}
		waitCQETimeoutSYS := syscall.NsecToTimespec(waitCQETimeout.Nanoseconds())

		cq := make([]*iouring.CompletionQueueEvent, ring.CQEntries())

	AGAIN:
		if ctxErr := ctx.Err(); ctxErr != nil {
			goto STOP
		}
		if vortex.stopped.Load() {
			goto STOP
		}
		// peek and submit
		if peeked := vortex.ops.PeekBatch(operations); peeked > 0 {
			prepared := int64(0)
			for i := int64(0); i < peeked; i++ {
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
			vortex.ops.Advance(prepared)
		}
		// wait
		if _, waitErr := ring.SubmitAndWaitTimeout(waitCQENr, &waitCQETimeoutSYS, nil); waitErr != nil {
			if errors.Is(waitErr, syscall.EAGAIN) || errors.Is(waitErr, syscall.EINTR) || errors.Is(waitErr, syscall.ETIME) {
				// decr waitCQENr and waitTimeout
				waitCQENr, waitCQETimeout = waitTransmission.Down()
				if waitCQENr < 1 {
					waitCQENr = defaultWaitCQENr
				}
				if waitCQETimeout < 1 {
					waitCQETimeout = defaultWaitTimeout
				}
				waitCQETimeoutSYS = syscall.NsecToTimespec(waitCQETimeout.Nanoseconds())
			}
			goto AGAIN
		}
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
					// sent result when op not done (when done means timeout or ctx canceled)
					var (
						res int
						err error
					)
					if cqe.Res < 0 {
						err = os.NewSyscallError(cop.Name(), syscall.Errno(-cqe.Res))
					} else {
						res = int(cqe.Res)
					}
					cop.setResult(res, err)
				} else { // done
					// 1. by timeout or ctx canceled, so should be hijacked
					// release
					vortex.releaseOperation(cop)
				}
			}
			// CQAdvance
			ring.CQAdvance(completed)
			// incr waitCQENr and waitTimeout
			waitCQENr, waitCQETimeout = waitTransmission.MatchN(completed)
			if waitCQENr < 1 {
				waitCQENr = defaultWaitCQENr
			}
			if waitCQETimeout < 1 {
				waitCQETimeout = defaultWaitTimeout
			}
			waitCQETimeoutSYS = syscall.NsecToTimespec(waitCQETimeout.Nanoseconds())
		}
		goto AGAIN

	STOP:
		// evict remain
		if remains := vortex.ops.Length(); remains > 0 {
			peeked := vortex.ops.PeekBatch(operations)
			for i := int64(0); i < peeked; i++ {
				op := operations[i]
				operations[i] = nil
				op.setResult(0, Uncompleted)
			}
		}

		// unlock os thread
		if vortex.lockOSThread {
			runtime.UnlockOSThread()
		}
		// done
		vortex.wg.Done()
	}(ctx, vortex)
}
