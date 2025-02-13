package ring

import (
	"context"
	"github.com/brickingsoft/errors"
	"github.com/pawelgaczynski/giouring"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

func New(size int) (*Ring, error) {
	if size <= 0 {
		size = 8 // todo
	}
	r, rErr := giouring.CreateRing(uint32(size))
	if rErr != nil {
		return nil, rErr
	}
	queue := NewOperationQueue(size)

	return &Ring{
		ring:  r,
		queue: queue,
	}, nil
}

type Ring struct {
	ring  *giouring.Ring
	queue *OperationQueue
}

func (ring *Ring) Push(op *Operation) bool {
	return ring.queue.Enqueue(op)
}

func (ring *Ring) Start(ctx context.Context) {
	go func(ring *Ring) {
		waitTimeout := syscall.NsecToTimespec((50 * time.Millisecond).Nanoseconds())
		operations := make([]*Operation, ring.queue.capacity)
		cqes := make([]*giouring.CompletionQueueEvent, ring.queue.capacity)
		zeroPeeked := 0
		stopped := false
		for {
			select {
			case <-ctx.Done():
				stopped = true
				break
			default:
				// peek
				peeked := ring.queue.PeekBatch(operations)
				if peeked == 0 {
					zeroPeeked++
					if zeroPeeked > 10 {
						runtime.Gosched()
					}
					time.Sleep(500 * time.Nanosecond)
					continue
				}
				// prepare
				prepared := int64(0)
				for i := int64(0); i < peeked; i++ {
					op := operations[i]
					if op == nil {
						break
					}
					sqe := ring.ring.GetSQE()
					if sqe == nil {
						break
					}
					op.prepare(sqe)
					prepared++
				}
				if prepared == 0 {
					continue
				}
				// submit
				for {
					if submitted, submitErr := ring.ring.SubmitAndWaitTimeout(1, &waitTimeout, nil); submitErr != nil {
						if errors.Is(submitErr, syscall.EAGAIN) || errors.Is(submitErr, syscall.EINTR) || errors.Is(submitErr, syscall.ETIME) {
							continue
						}
						for i := int64(0); i < prepared; i++ {
							op := operations[i]
							op.ch <- Result{Err: submitErr} // todo make err
						}
						ring.queue.Advance(prepared)
						break
					} else {
						n := submitted.Res
						ring.queue.Advance(int64(n))
						break
					}
				}

				// complete queue
				completed := ring.ring.PeekBatchCQE(cqes)
				if completed == 0 {
					continue
				}
				for i := uint32(0); i < completed; i++ {
					cqe := cqes[i]
					if cqe.UserData == 0 {
						continue
					}
					ch := *(*chan Result)(unsafe.Pointer(uintptr(cqe.UserData)))
					ch <- Result{
						N:   int(cqe.Res),
						Err: nil,
					}
				}
				if completed > 0 {
					ring.ring.CQAdvance(completed)
				}
			}
			if stopped {
				break
			}
		}
	}(ring)
}
