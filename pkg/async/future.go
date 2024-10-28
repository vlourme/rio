package async

import (
	"context"
	"errors"
)

var (
	ErrFutureClosed              = errors.New("rio: promise was closed")
	ErrFutureCancelled           = errors.New("rio: promise was cancelled")
	ErrGetPromiseFailedByTimeout = errors.New("rio: get promise timeout")
)

type Future[R any] interface {
	OnComplete(handler ResultHandler[R])
}

type futureImpl[R any] struct {
	ctx  context.Context
	ch   chan Result[R]
	exec ExecutorSubmitter
}

func (f *futureImpl[R]) OnComplete(handler ResultHandler[R]) {
	run := futureRunner[R]{
		ch:      f.ch,
		handler: handler,
	}
	f.exec.Submit(f.ctx, run)
}

func (f *futureImpl[R]) Complete(result R, err error) {
	if err == nil {
		f.Succeed(result)
	} else {
		f.Fail(err)
	}
}

func (f *futureImpl[R]) Succeed(result R) {
	f.ch <- newSucceedAsyncResult[R](result)
	close(f.ch)
}

func (f *futureImpl[R]) Fail(cause error) {
	f.ch <- newFailedAsyncResult[R](cause)
	close(f.ch)
}

func (f *futureImpl[R]) Future() (future Future[R]) {
	future = f
	return
}

type futureRunner[R any] struct {
	ch      chan Result[R]
	handler ResultHandler[R]
}

func (run futureRunner[R]) Run(ctx context.Context) {
	select {
	case <-ctx.Done():
		run.handler(ctx, *(new(R)), ErrFutureCancelled)
		return
	case ar, ok := <-run.ch:
		if !ok {
			run.handler(ctx, *(new(R)), ErrFutureClosed)
			return
		}
		if ar.Succeed() {
			run.handler(ctx, ar.Result(), nil)
		} else {
			run.handler(ctx, *(new(R)), ar.Cause())
		}
	}
}
