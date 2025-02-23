package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/kernel"
	"runtime"
	"sync"
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
	return errors.Is(err, Timeout)
}

func IsUnsupported(err error) bool {
	return errors.Is(err, UnsupportedOp)
}

type VortexOptions struct {
	Entries        uint32
	Flags          uint32
	Features       uint32
	WaitCQETimeout time.Duration
	WaitCQEBatches []uint32
}

func (options *VortexOptions) prepare() {
	if options.Entries == 0 {
		options.Entries = iouring.DefaultEntries
	}
	if options.Flags == 0 && options.Features == 0 {
		options.Flags, options.Features = DefaultIOURingFlagsAndFeatures()
	}
	if options.WaitCQETimeout < 1 {
		options.WaitCQETimeout = 50 * time.Millisecond
	}
	if len(options.WaitCQEBatches) == 0 {
		options.WaitCQEBatches = []uint32{1, 2, 4, 8, 16, 32, 64, 96, 128, 256, 384, 512, 768, 1024, 1536, 2048, 3072, 4096, 5120, 6144, 7168, 8192, 10240}
	}
}

const (
	minKernelVersionMajor = 5
	minKernelVersionMinor = 1
)

func NewVortex(options VortexOptions) (v *Vortex, err error) {
	ver, verErr := kernel.GetKernelVersion()
	if verErr != nil {
		return nil, verErr
	}
	target := kernel.Version{
		Kernel: ver.Kernel,
		Major:  minKernelVersionMajor,
		Minor:  minKernelVersionMinor,
		Flavor: ver.Flavor,
	}

	if kernel.CompareKernelVersion(*ver, target) < 0 {
		return nil, errors.New("kernel version too low")
	}

	options.prepare()
	// iouring
	ring, ringErr := iouring.New(options.Entries, options.Flags, options.Features, nil)
	if ringErr != nil {
		return nil, ringErr
	}
	sqEntries := ring.SQEntries()
	// ops
	ops := newOperationRing(int(sqEntries))
	// vortex
	v = &Vortex{
		ring:           ring,
		ops:            ops,
		lockOSThread:   options.Flags&iouring.SetupSingleIssuer != 0,
		waitCQETimeout: options.WaitCQETimeout,
		waitCQEBatches: options.WaitCQEBatches,
		operations: sync.Pool{
			New: func() interface{} {
				return &Operation{
					kind:     iouring.OpLast,
					borrowed: true,
					ch:       make(chan Result, 1),
				}
			},
		},
		timers: sync.Pool{
			New: func() interface{} {
				return time.NewTimer(0)
			},
		},
		hijackedOps: sync.Map{},
		stopCh:      nil,
		wg:          sync.WaitGroup{},
	}
	return
}

type Vortex struct {
	ring           *iouring.Ring
	ops            *OperationRing
	lockOSThread   bool
	waitCQETimeout time.Duration
	waitCQEBatches []uint32
	operations     sync.Pool
	timers         sync.Pool
	hijackedOps    sync.Map
	stopCh         chan struct{}
	wg             sync.WaitGroup
}

func (vortex *Vortex) Cancel(target *Operation) (ok bool) {
	if target.status.CompareAndSwap(ReadyOperationStatus, CompletedOperationStatus) || target.status.CompareAndSwap(ProcessingOperationStatus, CompletedOperationStatus) {
		op := &Operation{} // do not make ch cause no userdata
		op.PrepareCancel(target)
		pushed := false
		for i := 0; i < 10; i++ {
			if pushed = vortex.ops.Submit(op); pushed {
				time.Sleep(ns500)
				break
			}
		}
		runtime.KeepAlive(op)
		if pushed { // hijacked op
			vortex.hijackedOps.Store(op, struct{}{})
		} else { // hijacked target
			vortex.hijackedOps.Store(target, struct{}{})
		}
		ok = true
		return
	}
	return
}

func (vortex *Vortex) Close() (err error) {
	if vortex.stopCh != nil {
		close(vortex.stopCh)
		vortex.wg.Wait()
		err = vortex.ring.Close()
		vortex.hijackedOps.Clear()
		return
	}
	err = vortex.ring.Close()
	return
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

func (vortex *Vortex) acquireTimer(duration time.Duration) *time.Timer {
	timer := vortex.timers.Get().(*time.Timer)
	timer.Reset(duration)
	return timer
}

func (vortex *Vortex) releaseTimer(timer *time.Timer) {
	timer.Stop()
	vortex.timers.Put(timer)
}

func (vortex *Vortex) Start(ctx context.Context) {
	vortex.stopCh = make(chan struct{}, 1)
	vortex.wg.Add(1)
	go func(ctx context.Context, vortex *Vortex) {
		// lock os thread
		if vortex.lockOSThread {
			runtime.LockOSThread()
		}
		ring := vortex.ring

		stopCh := vortex.stopCh

		ops := vortex.ops
		operations := make([]*Operation, ops.capacity)

		waitCQENr := uint32(1)
		waitCQEBatchedIndex := uint32(0)
		waitCQEBatches := vortex.waitCQEBatches
		waitCQEBatchesLen := uint32(len(waitCQEBatches))
		waitCQETimeout := syscall.NsecToTimespec(vortex.waitCQETimeout.Nanoseconds())
		cq := make([]*iouring.CompletionQueueEvent, ops.capacity)

		stopped := false
		for {
			select {
			case <-ctx.Done():
				stopped = true
				break
			case <-stopCh:
				stopped = true
				break
			default:
				// peek and submit
				if peeked := ops.PeekBatch(operations); peeked > 0 {
					prepared := int64(0)
					for i := int64(0); i < peeked; i++ {
						op := operations[i]
						if op == nil {
							break
						}
						operations[i] = nil
						if op.status.CompareAndSwap(ReadyOperationStatus, ProcessingOperationStatus) {
							if ok, prepErr := vortex.prepareSQE(op); !ok {
								if prepErr != nil { // when prep err occur, means invalid op kind,
									op.ch <- Result{
										Err: prepErr,
									}
									prepared++ // prepareSQE nop whit out userdata, so prepared++
									continue
								}
								break
							}
							runtime.KeepAlive(op)
							prepared++
						} else { // maybe canceled
							vortex.hijackedOps.Delete(op)
							vortex.releaseOperation(op)
						}
					}
					// submit prepared
					if prepared > 0 {
						for {
							_, submitErr := ring.Submit()
							if submitErr != nil {
								if errors.Is(submitErr, syscall.EAGAIN) || errors.Is(submitErr, syscall.EINTR) || errors.Is(submitErr, syscall.ETIME) {
									time.Sleep(ns500)
									continue
								}
								break
							}
							ops.Advance(prepared)
							break
						}
					}
				}
				// wait
				if _, waitErr := ring.WaitCQEs(waitCQENr, &waitCQETimeout, nil); waitErr != nil {
					if errors.Is(waitErr, syscall.EAGAIN) || errors.Is(waitErr, syscall.EINTR) || errors.Is(waitErr, syscall.ETIME) {
						// decr waitCQENr
						if waitCQEBatchedIndex != 0 {
							waitCQEBatchedIndex--
							waitCQENr = waitCQEBatches[waitCQEBatchedIndex]
						}
					}
					continue
				}
				// peek cqe
				if completed := ring.PeekBatchCQE(cq); completed > 0 {
					for i := uint32(0); i < completed; i++ {
						cqe := cq[i]
						cq[i] = nil
						if cqe.UserData == 0 { // no userdata means no op
							continue
						}
						// get op from
						copPtr := cqe.GetData()
						cop := (*Operation)(copPtr)
						// handle
						if cop.status.CompareAndSwap(ProcessingOperationStatus, CompletedOperationStatus) { // not done
							// sent result when op not done (when done means timeout or ctx canceled)
							var (
								res   int
								err   error
								flags = cqe.Flags
							)
							if cqe.Res < 0 {
								err = syscall.Errno(-cqe.Res)
							} else {
								res = int(cqe.Res)
							}
							cop.ch <- Result{
								N:     res,
								Flags: flags,
								Err:   err,
							}
						} else { // done
							// 1. by timeout or ctx canceled, so should be hijacked
							// 2. by send_zc or sendmsg_zc, so should be hijacked
							// release hijacked
							if _, hijacked := vortex.hijackedOps.LoadAndDelete(cop); hijacked {
								vortex.releaseOperation(cop)
							}
						}
					}
					// CQAdvance
					ring.CQAdvance(completed)
					// incr waitCQENr
					for index := uint32(1); index < waitCQEBatchesLen; index++ {
						if waitCQEBatches[index] > completed {
							break
						}
						waitCQEBatchedIndex = index
					}
					waitCQENr = waitCQEBatches[waitCQEBatchedIndex]
				}
			}
			if stopped {
				break
			}
		}
		// evict remain
		if remains := ops.Len(); remains > 0 {
			peeked := ops.PeekBatch(operations)
			for i := int64(0); i < peeked; i++ {
				op := operations[i]
				operations[i] = nil
				op.ch <- Result{
					N:   0,
					Err: Uncompleted,
				}
			}
		}
		// done
		vortex.wg.Done()
		// unlock os thread
		if vortex.lockOSThread {
			runtime.UnlockOSThread()
		}
	}(ctx, vortex)
}
