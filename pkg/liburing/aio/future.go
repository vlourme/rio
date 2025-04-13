//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	promises = [2]sync.Pool{}
)

const (
	oneshotPromiseChSize   = 2
	multishotPromiseChSize = 1024
)

func acquireFuture(multishot bool) *operationFuture {
	if multishot {
		v := promises[1].Get()
		if v == nil {
			v = &operationFuture{
				ch:      make(chan CompletionEvent, multishotPromiseChSize),
				adaptor: nil,
				timeout: nil,
			}
		}
		return v.(*operationFuture)
	}
	v := promises[0].Get()
	if v == nil {
		v = &operationFuture{
			ch:      make(chan CompletionEvent, oneshotPromiseChSize),
			adaptor: nil,
			timeout: nil,
		}
	}
	return v.(*operationFuture)
}

func releaseFuture(future *operationFuture) {
	future.adaptor = nil
	future.timeout = nil
	future.hijacked = 0

	switch cap(future.ch) {
	case oneshotPromiseChSize:
		promises[0].Put(future)
		break
	case multishotPromiseChSize:
		promises[1].Put(future)
		break
	default:
		break
	}
}

type Promise interface {
	Future() Future
	Complete(n int, flags uint32, err error)
}

type CompletionEvent struct {
	N          int
	Flags      uint32
	Err        error
	Attachment unsafe.Pointer
}

// PromiseAdaptor
// todo
// AcceptPromiseAdaptor: attachment is member or event loop group
// ReceivePromiseAdaptor: attachment is buffer of bid bytes
type PromiseAdaptor interface {
	Handle(n int, flags uint32, err error) (int, uint32, unsafe.Pointer, error)
}

type Future interface {
	Await() (n int, flags uint32, attachment unsafe.Pointer, err error)
	AwaitDeadline(deadline time.Time) (n int, flags uint32, attachment unsafe.Pointer, err error)
	TryAwait() (n int, flags uint32, attachment unsafe.Pointer, awaited bool, err error)
	AwaitMore(hungry bool, deadline time.Time) (events []CompletionEvent)
}

type operationFuture struct {
	ch       chan CompletionEvent
	adaptor  PromiseAdaptor
	timeout  Future
	hijacked int
}

func (future *operationFuture) Await() (n int, flags uint32, attachment unsafe.Pointer, err error) {
	r, ok := <-future.ch
	if !ok {
		err = ErrCanceled
		return
	}
	n, flags, attachment, err = r.N, r.Flags, r.Attachment, r.Err
	if future.timeout != nil {
		if _, _, _, timeoutErr := future.timeout.Await(); errors.Is(timeoutErr, syscall.ETIME) {
			err = ErrTimeout
		}
	}
	return
}

func (future *operationFuture) AwaitDeadline(deadline time.Time) (n int, flags uint32, attachment unsafe.Pointer, err error) {
	if future.timeout != nil {
		panic(errors.New("future cannot await deadline when timeout is set"))
	}
	if deadline.IsZero() {
		return future.Await()
	}
	timeout := time.Until(deadline)
	if timeout < 1 {
		err = ErrTimeout
		return
	}
	timer := acquireTimer(timeout)
	select {
	case r := <-future.ch:
		n, flags, attachment, err = r.N, r.Flags, r.Attachment, r.Err
		break
	case <-timer.C:
		err = ErrTimeout
		break
	}
	releaseTimer(timer)
	return
}

func (future *operationFuture) TryAwait() (n int, flags uint32, attachment unsafe.Pointer, awaited bool, err error) {
	select {
	case r, ok := <-future.ch:
		if !ok {
			err = ErrCanceled
			return
		}
		n, flags, attachment, awaited, err = r.N, r.Flags, r.Attachment, true, r.Err
		break
	default:
		break
	}
	return
}

func (future *operationFuture) AwaitMore(hungry bool, deadline time.Time) (events []CompletionEvent) {
	ready := len(future.ch)
	if ready == 0 {
		if !hungry {
			return
		}
		n, flags, attachment, err := future.AwaitDeadline(deadline)
		event := CompletionEvent{
			N:          n,
			Flags:      flags,
			Err:        err,
			Attachment: attachment,
		}
		events = append(events, event)
		if err != nil {
			return
		}
		ready = len(future.ch)
		if ready == 0 {
			return
		}
	}
	for i := 0; i < ready; i++ {
		n, flags, attachment, err := future.Await()
		event := CompletionEvent{
			N:          n,
			Flags:      flags,
			Err:        err,
			Attachment: attachment,
		}
		events = append(events, event)
		if err != nil {
			return
		}
	}
	return
}

func (future *operationFuture) Complete(n int, flags uint32, err error) {
	if future.adaptor == nil {
		if err != nil {
			future.ch <- CompletionEvent{n, flags, err, nil}
			return
		}
		if flags&liburing.IORING_CQE_F_MORE != 0 {
			future.hijacked = n
			return
		}
		if flags&liburing.IORING_CQE_F_NOTIF != 0 {
			future.ch <- CompletionEvent{future.hijacked, flags, nil, nil}
			return
		}
		future.ch <- CompletionEvent{n, flags, err, nil}
		return
	}
	var (
		attachment unsafe.Pointer
	)
	n, flags, attachment, err = future.adaptor.Handle(n, flags, err)
	future.ch <- CompletionEvent{n, flags, err, attachment}
	return
}

func (future *operationFuture) Future() Future {
	return future
}
