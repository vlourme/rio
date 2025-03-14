//go:build linux

package aio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/process"
	"os"
	"runtime"
	"syscall"
)

func (r *Ring) waitingCQE(ctx context.Context) {
	defer r.wg.Done()

	// cpu affinity
	if r.waitAFFCPU > -1 {
		runtime.LockOSThread()
		if setErr := process.SetCPUAffinity(r.waitAFFCPU); setErr != nil {
			runtime.UnlockOSThread()
		} else {
			defer runtime.UnlockOSThread()
		}
	}

	ring := r.ring
	transmission := NewCurveTransmission(r.waitCQETimeCurve)
	cqeWaitMaxCount, cqeWaitTimeout := transmission.Up()
	waitCQBatchSize := r.waitCQEBatchSize
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
