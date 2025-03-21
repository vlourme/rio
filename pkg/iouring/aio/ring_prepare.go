//go:build linux

package aio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/process"
	"os"
	"runtime"
	"syscall"
	"time"
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
					op.setResult(0, 0, os.NewSyscallError("ring_getsqe", syscall.EBUSY)) // when prep err occur, means invalid op kind or no sqe left
					break
				}
				if makeErr := op.makeSQE(sqe); makeErr != nil { // make err but prep_nop, so need to submit
					op.setResult(0, 0, makeErr)
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
		batchIdx      = -1
		stopped       = false
		idle          = false
		needToPrepare = false
		needToSubmit  = 0
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
				sqe := ring.GetSQE()
				if sqe == nil {
					op.setResult(0, 0, os.NewSyscallError("ring_getsqe", syscall.EBUSY))
					break
				}
				if makeErr := op.makeSQE(sqe); makeErr != nil { //
					op.setResult(0, 0, makeErr)
				}
				batchIdx++
				batch[batchIdx] = sqe
				runtime.KeepAlive(sqe)
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
			needToPrepare = false
			received := batchIdx + 1

			submitted, _ := ring.Submit()
			needToSubmit += received - int(submitted)
			if needToSubmit < 0 {
				needToSubmit = 0
			}

			for i := 0; i < received; i++ {
				batch[i] = nil
			}
			batchIdx = -1

			// reset batch time window
			batchTimer.Reset(batchTimeWindow)
		}
	}
}
