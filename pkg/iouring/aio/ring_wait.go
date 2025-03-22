//go:build linux

package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"golang.org/x/sys/unix"
	"os"
	"syscall"
	"time"
)

func (r *Ring) waitingCQEWithPushMode(ctx context.Context) {
	defer r.wg.Done()

	eventFd := r.eventFd
	exitFd := r.exitFd
	epollFd := r.epollFd
	events := make([]unix.EpollEvent, 8)
	b := make([]byte, 8)

	ring := r.ring
	curve := r.waitCQETimeCurve
	if len(curve) == 0 {
		curve = defaultPushCurve
	}
	transmission := NewCurveTransmission(curve)
	cqeWaitMaxCount, cqeWaitTimeout := transmission.Up()
	cqes := make([]*iouring.CompletionQueueEvent, 1024)
	cqesLen := uint32(len(cqes))
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
				_, _ = unix.Read(eventFd, b)

				ready := ring.CQReady()
				if ready > cqesLen {
					cqesLen = iouring.RoundupPow2(ready)
					cqes = make([]*iouring.CompletionQueueEvent, cqesLen)
				}
				if peeked := ring.PeekBatchCQE(cqes); peeked > 0 {
					for j := uint32(0); j < peeked; j++ {
						cqe := cqes[j]
						cqes[j] = nil

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
						cop.complete(opN, opFlags, opErr)
					}
					ring.CQAdvance(peeked)
				}

				if ready > cqeWaitMaxCount {
					cqeWaitMaxCount, cqeWaitTimeout = transmission.Up()
				} else {
					cqeWaitMaxCount, cqeWaitTimeout = transmission.Down()
				}
				time.Sleep(cqeWaitTimeout)
				break
			default:
				// unknown fd
				_, _ = unix.Read(int(event.Fd), b)
				break
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

func (r *Ring) waitingCQEWithPullMode(ctx context.Context) {
	defer r.wg.Done()

	ring := r.ring
	if r.waitCQEPullIdleTime < 1 {
		r.waitCQEPullIdleTime = defaultWaitCQEPullIdleTime
	}
	idleTime := syscall.NsecToTimespec(r.waitCQEPullIdleTime.Nanoseconds())
	needToIdle := false
	curve := r.waitCQETimeCurve
	if len(curve) == 0 {
		curve = defaultPullCurve
	}
	transmission := NewCurveTransmission(curve)
	cqeWaitMaxCount, cqeWaitTimeout := transmission.Up()
	cqes := make([]*iouring.CompletionQueueEvent, 1024)
	cqesLen := uint32(len(cqes))
	waitZeroTimes := 5
	stopped := false
	for {
		select {
		case <-ctx.Done():
			stopped = true
			break
		default:
			ready := ring.CQReady()
			if ready > cqesLen {
				cqesLen = iouring.RoundupPow2(ready)
				cqes = make([]*iouring.CompletionQueueEvent, cqesLen)
			}
			if completed := ring.PeekBatchCQE(cqes); completed > 0 {
				for i := uint32(0); i < completed; i++ {
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
					cop.complete(opN, opFlags, opErr)
				}
				// CQAdvance
				ring.CQAdvance(completed)
			} else {
				if needToIdle {
					if _, waitErr := ring.WaitCQETimeout(&idleTime); waitErr != nil {
						if ctx.Err() != nil { // done
							break
						}
						break
					} else { // reset idle
						needToIdle = false
						waitZeroTimes = 10
					}
				}
				ns := syscall.NsecToTimespec(cqeWaitTimeout.Nanoseconds())
				if _, waitErr := ring.WaitCQEs(cqeWaitMaxCount, &ns, nil); waitErr != nil {
					if ctx.Err() != nil { // done
						break
					}
					cqeWaitMaxCount, cqeWaitTimeout = transmission.Down()
					waitZeroTimes--
					if waitZeroTimes < 1 {
						needToIdle = true
					}
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
