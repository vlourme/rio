package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"runtime"
	"sync"
	"syscall"
	"time"
)

var (
	Uncompleted   = errors.New("uncompleted")
	Timeout       = errors.New("timeout")
	UnsupportedOp = errors.New("unsupported op")
)

func IsUncompleted(err error) bool {
	return errors.Is(err, Uncompleted)
}

func IsTimeout(err error) bool {
	return errors.Is(err, Timeout)
}

func IsUnsupported(err error) bool {
	return errors.Is(err, UnsupportedOp)
}

func newVortex(entries uint32, flags uint32, features uint32, waitCQETimeout time.Duration, waitCQEBatches []uint32) (v *Vortex, err error) {
	// iouring
	ring, ringErr := iouring.New(entries, flags, features, nil)
	if ringErr != nil {
		return nil, ringErr
	}
	// queue
	queue := newOperationQueue(int(entries))
	// vortex
	v = &Vortex{
		ring:           ring,
		queue:          queue,
		lockOSThread:   flags&iouring.SetupSingleIssuer != 0,
		waitCQETimeout: waitCQETimeout,
		waitCQEBatches: waitCQEBatches,
		operations: sync.Pool{
			New: func() interface{} {
				return &Operation{
					ch: make(chan Result, 1),
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
	queue          *OperationQueue
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
		op := Operation{} // do not make ch cause no userdata
		op.PrepareCancel(target)
		pushed := false
		for i := 0; i < 10; i++ {
			if pushed = vortex.queue.Enqueue(&op); pushed {
				break
			}
		}
		if !pushed { // hijacked
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
	op.reset()
	vortex.operations.Put(op)
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

func (vortex *Vortex) start(ctx context.Context) {
	vortex.stopCh = make(chan struct{}, 1)
	vortex.wg.Add(1)
	go func(ctx context.Context, vortex *Vortex) {
		// unlock os thread
		if vortex.lockOSThread {
			runtime.LockOSThread()
		}
		ring := vortex.ring

		stopCh := vortex.stopCh

		queue := vortex.queue
		operations := make([]*Operation, queue.capacity)

		waitCQENr := uint32(1)
		waitCQEBatchedIndex := uint32(0)
		waitCQEBatches := vortex.waitCQEBatches
		waitCQEBatchesLen := uint32(len(waitCQEBatches))
		waitCQETimeout := syscall.NsecToTimespec(vortex.waitCQETimeout.Nanoseconds())
		cq := make([]*iouring.CompletionQueueEvent, queue.capacity)

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
				if peeked := queue.PeekBatch(operations); peeked > 0 {
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
									continue
								}
								break
							}
							queue.Advance(prepared)
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
		if remains := queue.Len(); remains > 0 {
			peeked := queue.PeekBatch(operations)
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
