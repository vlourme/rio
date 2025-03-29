//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

var (
	ErrIOURingSQBusy = errors.New("submission queue is busy")
)

func newOperationProducer(ring *liburing.Ring, producerLockOSThread bool, batchSize int, batchTimeWindow time.Duration, batchIdleTime time.Duration) *operationProducer {
	if ring.Flags()&liburing.IORING_SETUP_SINGLE_ISSUER != 0 {
		producerLockOSThread = true
	}
	if ring.Flags()&liburing.IORING_SETUP_SQPOLL == 0 && batchSize < 1 {
		batchSize = 64
	}

	p := &operationProducer{
		running: atomic.Bool{},
		ring:    ring,
		ch:      make(chan *Operation, ring.SQEntries()),
		wg:      new(sync.WaitGroup),
	}

	p.running.Store(true)

	if batchSize < 1 {
		go p.handleImmediately(producerLockOSThread)
	} else {
		go p.handleBatch(producerLockOSThread, batchSize, batchTimeWindow, batchIdleTime)
	}

	p.wg.Add(1)
	return p
}

type operationProducer struct {
	running atomic.Bool
	ring    *liburing.Ring
	ch      chan *Operation
	wg      *sync.WaitGroup
}

func (producer *operationProducer) Produce(op *Operation) bool {
	if producer.running.Load() {
		producer.ch <- op
		return true
	}
	return false
}

func (producer *operationProducer) Close() (err error) {
	producer.running.Store(false)
	time.Sleep(50 * time.Millisecond)
	close(producer.ch)
	producer.wg.Wait()
	return
}

func (producer *operationProducer) handleImmediately(producerLockOSThread bool) {
	defer producer.wg.Done()

	if producerLockOSThread {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
	}
	ring := producer.ring
	operations := producer.ch

	for {
		op, ok := <-operations
		if !ok {
			break
		}
		if op == nil {
			continue
		}
		if op.prepareAble() {
			sqe := ring.GetSQE()
			if sqe == nil {
				op.failed(ErrIOURingSQBusy) // when prep err occur, means no sqe left
				break
			}
			var timeoutSQE *liburing.SubmissionQueueEntry
			if op.timeout != nil {
				timeoutSQE = ring.GetSQE()
				if timeoutSQE == nil { // timeout but no sqe, then prep_nop and submit
					op.failed(ErrIOURingSQBusy)
					sqe.PrepareNop()
					_, _ = ring.Submit()
					break
				}
			}
			if err := op.packingSQE(sqe); err != nil { // make err but prep_nop, so need to submit
				op.failed(err)
				sqe.PrepareNop()
			} else {
				if timeoutSQE != nil { // prep_link_timeout
					timeoutOp := (*Operation)(op.addr2)
					if timeoutErr := timeoutOp.packingSQE(timeoutSQE); timeoutErr != nil {
						// should be ok
						panic(errors.New("packing timeout SQE failed: " + timeoutErr.Error()))
					}
				}
			}
			_, _ = ring.Submit()
		}
	}
}

const (
	defaultProduceBatchIdleTime   = 30 * time.Second
	defaultProduceBatchTimeWindow = 100 * time.Microsecond
)

func (producer *operationProducer) handleBatch(producerLockOSThread bool, batchSize int, batchTimeWindow time.Duration, batchIdleTime time.Duration) {
	defer producer.wg.Done()

	if producerLockOSThread {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
	}
	ring := producer.ring
	operations := producer.ch

	if batchIdleTime < 1 {
		batchIdleTime = defaultProduceBatchIdleTime
	}
	if batchTimeWindow < 1 {
		batchTimeWindow = defaultProduceBatchTimeWindow
	}

	batchTimer := time.NewTimer(batchTimeWindow)
	defer batchTimer.Stop()

	batchOps := make([]*Operation, batchSize)
	batchSQEs := make([]*liburing.SubmissionQueueEntry, batchSize*2)

	var (
		batchIdx     = 0
		stopped      = false
		idle         = false
		needToSubmit = false
	)

	for {
		if stopped {
			break
		}
		select {
		case <-batchTimer.C:
			needToSubmit = true
			break
		case op, ok := <-operations:
			if !ok {
				stopped = true
				break
			}
			if op == nil {
				break
			}
			if idle {
				idle = false
				batchTimer.Reset(batchTimeWindow)
			}
			batchOps[batchIdx] = op
			batchIdx++
			if batchIdx == batchSize {
				needToSubmit = true
				break
			}
			break
		}
		if batchIdx == 0 { // when no request, use idle time
			idle = true
			batchTimer.Reset(batchIdleTime)
			continue
		}
		if needToSubmit { // go to prepare
			sqeIndex := 0
			for i := 0; i < batchIdx; i++ {
				op := batchOps[i]
				if op == nil {
					continue
				}
				if op.prepareAble() {
					sqe := ring.GetSQE()
					if sqe == nil {
						op.failed(ErrIOURingSQBusy) // when prep err occur, means no sqe left
						continue
					}
					var timeoutSQE *liburing.SubmissionQueueEntry
					if op.timeout != nil {
						timeoutSQE = ring.GetSQE()
						if timeoutSQE == nil { // timeout but no sqe, then prep_nop and submit
							op.failed(ErrIOURingSQBusy)
							sqe.PrepareNop()
							batchSQEs[sqeIndex] = sqe
							sqeIndex++
							continue
						}
					}
					batchSQEs[sqeIndex] = sqe
					sqeIndex++
					if err := op.packingSQE(sqe); err != nil { // make err but prep_nop, so need to submit
						op.failed(err)
						sqe.PrepareNop()
					} else {
						if timeoutSQE != nil { // prep_link_timeout
							timeoutOp := (*Operation)(op.addr2)
							if timeoutErr := timeoutOp.packingSQE(timeoutSQE); timeoutErr != nil {
								// should be ok
								panic(errors.New("packing timeout SQE failed: " + timeoutErr.Error()))
							}
							batchSQEs[sqeIndex] = timeoutSQE
							sqeIndex++
						}
					}
					continue
				}
			}

		SUBMIT:
			if _, submitErr := ring.Submit(); submitErr != nil {
				if errors.Is(submitErr, syscall.EAGAIN) || errors.Is(submitErr, syscall.EINTR) {
					goto SUBMIT
				}
				for i := 0; i < batchIdx; i++ {
					op := batchOps[i]
					op.failed(os.NewSyscallError("ring_submit", submitErr))
				}
				continue
			}
			// clean
			for i := 0; i < batchIdx; i++ {
				batchOps[i] = nil
			}
			for i := 0; i < sqeIndex; i++ {
				batchSQEs[sqeIndex] = nil
			}
			// reset
			batchIdx = 0
			batchTimer.Reset(batchTimeWindow)
		}
	}
}
