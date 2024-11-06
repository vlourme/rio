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
}

type Awaitable[R any] interface {
	Await() (v R, err error)
}

func Await[R any](future Future[R]) (v R, err error) {
	awaitable, ok := future.(Awaitable[R])
	if !ok {
		err = errors.New("rio: future is not a Awaitable[R]")
		return
	}
	v, err = awaitable.Await()
	return
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

func newFuture[R any](ctx context.Context, infinite bool, submitter ExecutorSubmitter) *futureImpl[R] {
	futureCtx, futureCtxCancel := context.WithCancel(ctx)
	buf := 1
	if infinite {
		buf = infiniteResultChanBufferSize
	}
	return &futureImpl[R]{
		ctx:                     ctx,
		futureCtx:               futureCtx,
		futureCtxCancel:         futureCtxCancel,
		futureDeadlineCtxCancel: nil,
		infinite:                infinite,
		rch:                     newResultChan[R](buf),
		submitter:               submitter,
	}
}

type futureImpl[R any] struct {
	ctx                     context.Context
	futureCtx               context.Context
	futureCtxCancel         context.CancelFunc
	futureDeadlineCtxCancel context.CancelFunc
	infinite                bool
	rch                     *resultChan[R]
	submitter               ExecutorSubmitter
}

func (f *futureImpl[R]) OnComplete(handler ResultHandler[R]) {
	run := futureRunner[R]{
		ctx:            f.futureCtx,
		cancel:         f.futureCtxCancel,
		deadlineCancel: f.futureDeadlineCtxCancel,
		infinite:       f.infinite,
		rch:            f.rch,
		handler:        handler,
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
		ctx:            f.futureCtx,
		cancel:         f.futureCtxCancel,
		deadlineCancel: f.futureDeadlineCtxCancel,
		infinite:       f.infinite,
		rch:            f.rch,
		handler:        handler,
	}
	f.submitter.Submit(f.ctx, run)
	ar := <-ch
	v = ar.Result()
	err = ar.Cause()
	return
}

func (f *futureImpl[R]) Complete(result R, err error) {
	f.rch.Emit(newAsyncResult[R](result, err))
	if !f.infinite {
		f.rch.Close()
	}
}

func (f *futureImpl[R]) Succeed(result R) {
	f.rch.Emit(newSucceedAsyncResult[R](result))
	if !f.infinite {
		f.rch.Close()
	}
}

func (f *futureImpl[R]) Fail(cause error) {
	f.rch.Emit(newFailedAsyncResult[R](cause))
	if !f.infinite {
		f.rch.Close()
	}
}

func (f *futureImpl[R]) Cancel() {
	if f.infinite {
		f.rch.Close()
	}
	f.futureCtxCancel()
	if f.futureDeadlineCtxCancel != nil {
		f.futureDeadlineCtxCancel()
	}
}

func (f *futureImpl[R]) SetDeadline(t time.Time) {
	if !f.infinite {
		f.futureCtx, f.futureDeadlineCtxCancel = context.WithDeadline(f.futureCtx, t)
	}
}

func (f *futureImpl[R]) Future() (future Future[R]) {
	future = f
	return
}

func (f *futureImpl[R]) Close() {
	f.rch.Close()
}

type futureRunner[R any] struct {
	ctx            context.Context
	cancel         context.CancelFunc
	deadlineCancel context.CancelFunc
	infinite       bool
	rch            *resultChan[R]
	handler        ResultHandler[R]
}

func (run futureRunner[R]) Run(ctx context.Context) {
	futureCtx := run.ctx
	futureCtxCancel := run.cancel
	rch := run.rch
	stopped := false
	for {
		select {
		case <-ctx.Done():
			if !run.infinite {
				rch.CloseUnexpectedly()
				run.handler(ctx, *(new(R)), ctx.Err())
				stopped = true
			}
			break
		case <-futureCtx.Done():
			if !run.infinite {
				rch.CloseUnexpectedly()
				run.handler(ctx, *(new(R)), futureCtx.Err())
				stopped = true
			}
			break
		case ar, ok := <-rch.ch:
			if !ok {
				run.handler(ctx, *(new(R)), ErrFutureWasClosed)
				stopped = true
				break
			}
			if ar.Succeed() {
				run.handler(ctx, ar.Result(), nil)
			} else {
				run.handler(ctx, *(new(R)), ar.Cause())
			}
			if !run.infinite {
				stopped = true
			}
			break
		}
		if stopped {
			break
		}
	}
	futureCtxCancel()
	if run.deadlineCancel != nil {
		run.deadlineCancel()
	}
}
