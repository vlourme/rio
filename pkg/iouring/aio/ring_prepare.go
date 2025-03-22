//go:build linux

package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/process"
	"runtime"
	"time"
)

var (
	ErrSQBusy = errors.New("submission queue is busy")
)

func (r *Ring) preparingSQEWithSQPollMode(ctx context.Context) {
	defer r.wg.Done()

	// cpu affinity
	if r.prepSQEAFFCPU > -1 {
		runtime.LockOSThread()
		_ = process.SetCPUAffinity(r.prepSQEAFFCPU)
		defer runtime.UnlockOSThread()
	}

	ring := r.ring
	requestCh := r.requestCh
	stopped := false
	for {
		select {
		case <-ctx.Done():
			stopped = true
			break
		case op, ok := <-requestCh:
			if !ok {
				stopped = true
				break
			}
			if op == nil {
				break
			}
			if op.canPrepare() {
				sqe := ring.GetSQE()
				if sqe == nil {
					op.failed(ErrSQBusy) // when prep err occur, means invalid op kind or no sqe left
					break
				}
				var timeoutSQE *iouring.SubmissionQueueEntry
				if op.timeout != nil {
					timeoutSQE = ring.GetSQE()
					if timeoutSQE == nil { // timeout1 but no sqe, then prep_nop and submit
						op.failed(ErrSQBusy)
						sqe.PrepareNop()
						_, _ = ring.Submit()
						break
					}
				}
				if err := op.packingSQE(sqe); err != nil { // make err but prep_nop, so need to submit
					op.failed(err)
				} else {
					if timeoutSQE != nil { // prep_link_timeout
						timeoutOP := NewOperation(1)
						timeoutOP.prepareLinkTimeout(op)
						timeoutOP.Prepared()
						if timeoutErr := timeoutOP.packingSQE(timeoutSQE); timeoutErr != nil {
							// should be ok
							panic(errors.New("packing timeout SQE failed: " + timeoutErr.Error()))
						}
					}
				}
				_, _ = ring.Submit()
			}
		}
		if stopped {
			break
		}
	}
}

func (r *Ring) preparingSQEWithBatchMode(ctx context.Context) {
	defer r.wg.Done()

	// cpu affinity
	if r.prepSQEAFFCPU > -1 {
		runtime.LockOSThread()
		_ = process.SetCPUAffinity(r.prepSQEAFFCPU)
		defer runtime.UnlockOSThread()
	}

	ring := r.ring
	idleTime := r.prepSQEIdleTime
	if idleTime < 1 {
		idleTime = defaultPrepSQEBatchIdleTime
	}
	batchTimeWindow := r.prepSQETimeWindow
	if batchTimeWindow < 1 {
		batchTimeWindow = defaultPrepSQEBatchTimeWindow
	}

	batchTimer := time.NewTimer(batchTimeWindow)
	defer batchTimer.Stop()

	minBatch := r.prepSQEMinBatch
	if minBatch < 1 {
		minBatch = 64
	}
	batch := make([]*iouring.SubmissionQueueEntry, minBatch)

	requestCh := r.requestCh

	var (
		batchIdx                 = -1
		stopped                  = false
		idle                     = false
		needToPrepare            = false
		needToSubmit             = 0
		overflowed    *Operation = nil
	)

	for {
		select {
		case <-ctx.Done():
			stopped = true
			break
		case <-batchTimer.C:
			needToPrepare = true
			break
		case op, ok := <-requestCh:
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
			if op.canPrepare() {
				if op.timeout != nil && uint32(batchIdx+2) >= minBatch { // timeout1 need twe sqe, when no remains then prepare first
					overflowed = op
					needToPrepare = true
					break
				}
				batchIdx = packingBatchOperation(ring, op, batchIdx, &batch)
				if uint32(batchIdx+1) == minBatch { // full so flush
					needToPrepare = true
				}
				break
			}
			break
		}
		if stopped { // check stopped
			break
		}
		if batchIdx == -1 { // when no request, use idle time
			idle = true
			batchTimer.Reset(idleTime)
			continue
		}

		if needToPrepare { // go to prepare
			if overflowed != nil && overflowed.canPrepare() {
				batchIdx = packingBatchOperation(ring, overflowed, batchIdx, &batch)
			}
			needToPrepare = false
			packed := batchIdx + 1

			submitted, _ := ring.Submit()
			needToSubmit += packed - int(submitted)
			if needToSubmit < 0 {
				needToSubmit = 0
			}

			for i := 0; i < packed; i++ {
				batch[i] = nil
			}
			batchIdx = -1

			// reset batch time window
			batchTimer.Reset(batchTimeWindow)
		}
	}
}

func packingBatchOperation(ring *iouring.Ring, op *Operation, batchIdx int, batchPtr *[]*iouring.SubmissionQueueEntry) int {
	sqe := ring.GetSQE()
	if sqe == nil {
		op.failed(ErrSQBusy)
		return batchIdx
	}
	batch := *batchPtr
	// handle timeout1
	var timeoutSQE *iouring.SubmissionQueueEntry
	if op.timeout != nil { // has timeout1 then get sqe for link timeout1
		timeoutSQE = ring.GetSQE()
		if timeoutSQE == nil { // no sqe then prep_nop
			sqe.PrepareNop()
			batchIdx++
			batch[batchIdx] = sqe
			op.failed(ErrSQBusy)
			return batchIdx
		}
	}
	if err := op.packingSQE(sqe); err != nil { // packing failed then prep_nop
		sqe.PrepareNop()
		batchIdx++
		batch[batchIdx] = sqe
		op.failed(err)
		return batchIdx
	}
	batchIdx++
	batch[batchIdx] = sqe
	// check has timeout1
	if timeoutSQE != nil { // prep_link_timeout
		timeoutOP := NewOperation(1)
		timeoutOP.prepareLinkTimeout(op)
		timeoutOP.Prepared()
		if timeoutErr := timeoutOP.packingSQE(timeoutSQE); timeoutErr != nil {
			// should be ok
			panic(errors.New("packing timeout SQE failed: " + timeoutErr.Error()))
		}
		batchIdx++
		batch[batchIdx] = timeoutSQE
	}
	runtime.KeepAlive(sqe)
	return batchIdx
}
