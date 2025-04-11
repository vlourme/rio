//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"syscall"
	"unsafe"
)

type Promise interface {
	Future() Future
	Complete(n int, flags uint32, err error)
}

type Future interface {
	Await() (n int, flags uint32, attachment unsafe.Pointer, err error)
	LinkTimeout(f Future)
}

type FutureResult struct {
	N          int
	Flags      uint32
	Err        error
	Attachment unsafe.Pointer
}

type baseFuture struct {
	ch      chan FutureResult
	timeout Future
}

func (future *baseFuture) Await() (n int, flags uint32, attachment unsafe.Pointer, err error) {
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

func (future *baseFuture) LinkTimeout(f Future) {
	future.timeout = f
}

type OneshotFuture struct {
	baseFuture
}

func (future *OneshotFuture) Future() Future {
	return future
}

func (future *OneshotFuture) Complete(n int, flags uint32, err error) {
	future.ch <- FutureResult{n, flags, err, nil}
}

type ZeroCopyFuture struct {
	baseFuture
	result FutureResult
}

func (future *ZeroCopyFuture) Future() Future {
	return future
}

func (future *ZeroCopyFuture) Complete(n int, flags uint32, err error) {
	if err != nil {
		future.ch <- FutureResult{n, flags, err, nil}
		return
	}
	if flags&liburing.IORING_CQE_F_MORE != 0 {
		future.result.N = n
		future.result.Flags = flags
		future.result.Err = nil
		return
	}
	if flags&liburing.IORING_CQE_F_NOTIF != 0 {
		future.ch <- future.result
	}
}

// MultishotPromiseAdaptor
// todo
// AcceptAdaptor: attachment is member or event loop group
// ReceiveAdaptor: attachment is buffer of bid bytes
type MultishotPromiseAdaptor interface {
	Handle(n int, flags uint32, err error) (r FutureResult)
}

type MultishotFuture struct {
	baseFuture
	adaptor MultishotPromiseAdaptor
}

func (future *MultishotFuture) Future() Future {
	return future
}

func (future *MultishotFuture) Complete(n int, flags uint32, err error) {
	if future.adaptor != nil {
		r := future.adaptor.Handle(n, flags, err)
		future.ch <- r
		return
	}
	future.ch <- FutureResult{n, flags, err, nil}
	return
}

func (future *MultishotFuture) SetAdaptor(adaptor MultishotPromiseAdaptor) {
	future.adaptor = adaptor
}
