//go:build linux

package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/process"
	"runtime"
	"syscall"
	"time"
)

func (r *Ring) preparingSQE(ctx context.Context) {
	defer r.wg.Done()

	// cpu affinity
	if r.ring.Flags()&iouring.SetupSingleIssuer != 0 || r.prepAFFCPU > -1 {
		if r.prepAFFCPU == -1 {
			r.prepAFFCPU = 0
		}
		runtime.LockOSThread()
		if setErr := process.SetCPUAffinity(r.prepAFFCPU); setErr != nil {
			if r.ring.Flags()&iouring.SetupSingleIssuer != 0 { // lock os thread is required
				defer runtime.UnlockOSThread()
			} else { // no single issuer so unlock
				runtime.UnlockOSThread()
			}
		} else {
			defer runtime.UnlockOSThread()
		}
	}

	ring := r.ring
	idleTime := r.prepSQEIdleTime
	if idleTime < 1 {
		idleTime = defaultPrepSQEBatchIdleTime
	}
	batchTimeWindow := r.prepSQEBatchTimeWindow
	if batchTimeWindow < 1 {
		batchTimeWindow = defaultPrepSQEBatchTimeWindow
	}

	batchTimer := time.NewTimer(batchTimeWindow)
	defer batchTimer.Stop()

	batchSize := r.prepSQEBatchSize
	if batchSize < 1 {
		batchSize = 1024
	}
	batch := make([]*Operation, batchSize) // todo delete batch

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
			batchIdx++
			batch[batchIdx] = op
			if uint32(batchIdx+1) == batchSize { // full so flush
				needToPrepare = true
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
			prepared := 0
			received := batchIdx + 1
			for i := 0; i < received; i++ {
				op := batch[i]
				batchIdx--
				batch[i] = nil
				if op.canPrepare() {
					if prepErr := op.makeSQE(r); prepErr != nil { // when prep err occur, means invalid op kind or no sqe left
						op.setResult(0, 0, prepErr)
						if errors.Is(prepErr, syscall.EBUSY) { // no sqe left
							if next := i + 1; next < received { // not last, so keep unprepared
								tmp := make([]*Operation, batchSize)
								copy(tmp, batch[next:])
								batch = tmp
							}
							break
						}
						prepared++ // prepareSQE nop whit out userdata, so prepared++
						continue
					}
					prepared++
				}
			}

			if prepared > 0 || needToSubmit > 0 { // Submit
				submitted, _ := ring.Submit()
				needToSubmit += prepared - int(submitted)
				if needToSubmit < 0 {
					needToSubmit = 0
				}
			}

			// reset batch time window
			batchTimer.Reset(batchTimeWindow)
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
}
