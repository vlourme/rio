package async

import (
	"context"
	"sync"
)

type Result[R any] interface {
	Succeed() (succeed bool)
	Failed() (failed bool)
	Result() (result R)
	Cause() (err error)
}

func newAsyncResult[R any](result R, cause error) Result[R] {
	return &resultImpl[R]{
		result: result,
		cause:  cause,
	}
}

func newSucceedAsyncResult[R any](result R) Result[R] {
	return &resultImpl[R]{
		result: result,
		cause:  nil,
	}
}

func newFailedAsyncResult[R any](cause error) Result[R] {
	return &resultImpl[R]{
		cause: cause,
	}
}

type resultImpl[R any] struct {
	result R
	cause  error
}

func (ar *resultImpl[R]) Succeed() (succeed bool) {
	succeed = ar.cause == nil
	return
}

func (ar *resultImpl[R]) Failed() (failed bool) {
	failed = ar.cause != nil
	return
}

func (ar *resultImpl[R]) Result() (result R) {
	result = ar.result
	return
}

func (ar *resultImpl[R]) Cause() (err error) {
	err = ar.cause
	return
}

type ResultHandler[R any] func(ctx context.Context, result R, err error)

func newResultChan[R any]() *resultChan[R] {
	return &resultChan[R]{
		ch:     make(chan Result[R], 1),
		closed: false,
		locker: sync.Mutex{},
	}
}

type resultChan[R any] struct {
	ch     chan Result[R]
	closed bool
	locker sync.Mutex
}

func (rch *resultChan[R]) Emit(r Result[R]) {
	rch.locker.Lock()
	if rch.closed {
		rch.locker.Unlock()
		return
	}
	rch.ch <- r
	close(rch.ch)
	rch.closed = true
	rch.locker.Unlock()
	return
}

func (rch *resultChan[R]) CloseUnexpectedly() {
	rch.locker.Lock()
	if rch.closed {
		rch.locker.Unlock()
		return
	}
	close(rch.ch)
	for {
		_, ok := <-rch.ch
		if !ok {
			break
		}
	}
	rch.closed = true
	rch.locker.Unlock()
}
