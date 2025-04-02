//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/liburing"
	"os"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

type OperationHandler interface {
	Handle(n int, flags uint32, err error)
}

func newOperationConsumer(ring *liburing.Ring, curve Curve) *operationConsumer {
	// curve
	if len(curve) == 0 {
		curve = Curve{
			{1, 15 * time.Second},
			{32, 100 * time.Microsecond},
			{64, 200 * time.Microsecond},
			{96, 500 * time.Microsecond},
		}
	}

	c := &operationConsumer{
		ring:          ring,
		wg:            new(sync.WaitGroup),
		waitTimeCurve: curve,
	}

	c.closeUserdata = uint64(uintptr(unsafe.Pointer(c)))

	go c.handle()
	c.wg.Add(1)
	return c
}

type operationConsumer struct {
	ring          *liburing.Ring
	wg            *sync.WaitGroup
	closeUserdata uint64
	waitTimeCurve Curve
}

func (c *operationConsumer) handle() {
	defer c.wg.Done()

	var (
		ring            = c.ring
		cqes            = make([]*liburing.CompletionQueueEvent, 1024)
		transmission    = NewCurveTransmission(c.waitTimeCurve)
		stopped         = false
		cqeWaitMaxCount uint32
		cqeWaitTimeout  *syscall.Timespec
		completed       uint32
	)

	for {
		if stopped {
			break
		}
		if peeked := ring.PeekBatchCQE(cqes); peeked > 0 {
			for i := uint32(0); i < peeked; i++ {
				cqe := cqes[i]
				cqes[i] = nil

				if cqe.UserData == 0 { // no userdata means no op
					ring.CQAdvance(1)
					continue
				}
				if cqe.IsInternalUpdateTimeoutUserdata() { // userdata means not op
					ring.CQAdvance(1)
					continue
				}
				if cqe.UserData == c.closeUserdata {
					stopped = true
					ring.CQAdvance(1)
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

				ring.CQAdvance(1)
			}
			// mark completed
			completed += peeked
			continue
		}
		// wait more cqe
		cqeWaitMaxCount, cqeWaitTimeout = transmission.Match(completed)
		_, _ = ring.WaitCQEs(cqeWaitMaxCount, cqeWaitTimeout, nil)
		// reset completed
		completed = 0
	}
	return
}

func (c *operationConsumer) Close() (err error) {
	for i := 0; i < 10; i++ {
		sqe := c.ring.GetSQE()
		if sqe == nil {
			_, _ = c.ring.Submit()
			time.Sleep(1 * time.Microsecond)
			continue
		}
		sqe.PrepareNop()
		sqe.SetData64(c.closeUserdata)
		if _, submitErr := c.ring.Submit(); submitErr != nil {
			continue
		}
		break
	}
	c.wg.Wait()
	return
}
