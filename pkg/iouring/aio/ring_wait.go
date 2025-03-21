//go:build linux

package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"golang.org/x/sys/unix"
	"os"
	"syscall"
)

func (r *Ring) waitingCQEWithEventMode(ctx context.Context) {
	defer r.wg.Done()

	eventFd := r.eventFd
	exitFd := r.exitFd
	epollFd := r.epollFd
	events := make([]unix.EpollEvent, 8)
	b := make([]byte, 8)

	ring := r.ring
	waitCQBatchSize := r.waitCQEBatchSize
	if waitCQBatchSize < 1 {
		waitCQBatchSize = 1024
	}
	cqes := make([]*iouring.CompletionQueueEvent, waitCQBatchSize)
	stopped := false
	for {
		n, waitErr := unix.EpollWait(epollFd, events, -1)
		if waitErr != nil {
			if errors.Is(waitErr, unix.EINTR) {
				continue
			}
			return
		}

		for i := 0; i < n; i++ {
			event := &events[i]
			switch event.Fd {
			case int32(exitFd):
				_, _ = unix.Read(exitFd, b)
				stopped = true
				break
			case int32(eventFd):
				if peeked := ring.PeekBatchCQE(cqes); peeked > 0 {
					for i := uint32(0); i < peeked; i++ {
						cqe := cqes[i]
						cqes[i] = nil

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
					ring.CQAdvance(peeked)
				}
				break
			default:
				// unknown fd
				_, _ = unix.Read(int(event.Fd), b)
				break
			}
			if event.Fd == int32(exitFd) {
				_, _ = unix.Read(exitFd, b)
				stopped = true
			}
			if event.Fd == int32(eventFd) {
				_, _ = unix.Read(exitFd, b)
			}
		}
		if stopped {
			break
		}
		if ctxErr := ctx.Err(); ctxErr != nil && errors.Is(ctxErr, context.Canceled) {
			break
		}
	}
}

func (r *Ring) waitingCQEWithBatchMode(ctx context.Context) {
	defer r.wg.Done()

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
