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
	ring   *giouring.Ring
	queue  *OperationQueue
	stopCh chan struct{}
}

func (ring *Ring) Push(op *Operation) bool {
	return ring.queue.Enqueue(op)
}

func (ring *Ring) Start(ctx context.Context) {
	ring.stopCh = make(chan struct{}, 1)
	go func(ctx context.Context, ring *Ring) {
		waitTimeout := syscall.NsecToTimespec((50 * time.Millisecond).Nanoseconds()) // todo wait timeout
		operations := make([]*Operation, ring.queue.capacity)
		cqes := make([]*giouring.CompletionQueueEvent, ring.queue.capacity)
		zeroPeeked := 0
		cqReady := false
		stopped := false
		for {
			select {
			case <-ctx.Done():
				stopped = true
				break
			case <-ring.stopCh:
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
					operations[i] = nil
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
					_, submitErr := ring.ring.Submit()
					if submitErr != nil {
						if errors.Is(submitErr, syscall.EAGAIN) || errors.Is(submitErr, syscall.EINTR) || errors.Is(submitErr, syscall.ETIME) {
							continue
						}
						break
					}
					ring.queue.Advance(prepared)
					break
				}
				// wait cqe
				for {
					_, waitErr := ring.ring.WaitCQEs(1, &waitTimeout, nil)
					if waitErr != nil {
						if errors.Is(waitErr, syscall.EAGAIN) {
							continue
						}
						cqReady = false
						break
					}
					cqReady = true
					break
				}
				if !cqReady {
					continue
				}
				// peek cqe
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
		// send failed for remains
		if remains := ring.queue.Len(); remains > 0 {
			peeked := ring.queue.PeekBatch(operations)
			for i := int64(0); i < peeked; i++ {
				op := operations[i]
				operations[i] = nil
				op.ch <- Result{
					N:   0,
					Err: errors.New("uncompleted via closed"),
				}
			}
		}
		// queue exit
		ring.ring.QueueExit()
	}(ctx, ring)
}

func (ring *Ring) Stop() {
	if ring.stopCh != nil {
		close(ring.stopCh)
		return
	}
	ring.ring.QueueExit()
	return
}
