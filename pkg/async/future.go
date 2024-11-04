package async

import (
	"context"
	"errors"
	"time"
)

var (
	ErrFutureWasClosed = errors.New("rio: promise was closed")
)

type Future[R any] interface {
	OnComplete(handler ResultHandler[R])
	Await() (v R, err error)
}

func SucceedImmediately[R any](ctx context.Context, value R) (f Future[R]) {
	f = &immediatelyFuture[R]{
		ctx:    ctx,
		result: value,
		cause:  nil,
	}
	return
}

func FailedImmediately[R any](ctx context.Context, cause error) (f Future[R]) {
	f = &immediatelyFuture[R]{
		ctx:    ctx,
		result: *(new(R)),
		cause:  cause,
	}
	return
}

type immediatelyFuture[R any] struct {
	ctx    context.Context
	result R
	cause  error
}

func (f *immediatelyFuture[R]) OnComplete(handler ResultHandler[R]) {
	handler(f.ctx, f.result, f.cause)
	return
}

func (f *immediatelyFuture[R]) Await() (v R, err error) {
	v, err = f.result, f.cause
	return
}

type futureImpl[R any] struct {
	ctx       context.Context
	cancel    context.CancelFunc
	rch       *resultChan[R]
	submitter ExecutorSubmitter
}

func (f *futureImpl[R]) OnComplete(handler ResultHandler[R]) {
	run := futureRunner[R]{
		rch:     f.rch,
		handler: handler,
	}
	f.submitter.Submit(f.ctx, run)
}

func (f *futureImpl[R]) Await() (v R, err error) {
	ch := make(chan Result[R], 1)
	var handler ResultHandler[R] = func(ctx context.Context, result R, err error) {
		ch <- newAsyncResult[R](result, err)
		close(ch)
	}
	run := futureRunner[R]{
		rch:     f.rch,
		handler: handler,
	}
	f.submitter.Submit(f.ctx, run)
	ar := <-ch
	v = ar.Result()
	err = ar.Cause()
	return
}

func (f *futureImpl[R]) Complete(result R, err error) {
	f.rch.Emit(newAsyncResult[R](result, err))
}

func (f *futureImpl[R]) Succeed(result R) {
	f.rch.Emit(newSucceedAsyncResult[R](result))
}

func (f *futureImpl[R]) Fail(cause error) {
	f.rch.Emit(newFailedAsyncResult[R](cause))
}

func (f *futureImpl[R]) Cancel() {
	f.cancel()
}

func (f *futureImpl[R]) SetDeadline(t time.Time) {
	f.ctx, f.cancel = context.WithDeadline(f.ctx, t)
}

func (f *futureImpl[R]) Future() (future Future[R]) {
	future = f
	return
}

type futureRunner[R any] struct {
	rch     *resultChan[R]
	handler ResultHandler[R]
}

func (run futureRunner[R]) Run(ctx context.Context) {
	rch := run.rch
	select {
	case <-ctx.Done():
		rch.CloseUnexpectedly()
		run.handler(ctx, *(new(R)), ctx.Err())
		return
	case ar, ok := <-rch.ch:
		if !ok {
			run.handler(ctx, *(new(R)), ErrFutureWasClosed)
			return
		}
		if ar.Succeed() {
			run.handler(ctx, ar.Result(), nil)
		} else {
			run.handler(ctx, *(new(R)), ar.Cause())
		}
	}
}
