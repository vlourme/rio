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

type futureImpl[R any] struct {
	ctx       context.Context
	cancel    context.CancelFunc
	rch       chan Result[R]
	submitter ExecutorSubmitter
}

func (f *futureImpl[R]) OnComplete(handler ResultHandler[R]) {
	run := futureRunner[R]{
		rch:     f.rch,
		handler: handler,
	}
	f.submitter.Submit(f.ctx, run)
}

func (f *futureImpl[R]) Complete(result R, err error) {
	if err == nil {
		f.Succeed(result)
	} else {
		f.Fail(err)
	}
}

func (f *futureImpl[R]) Succeed(result R) {
	f.rch <- newSucceedAsyncResult[R](result)
	close(f.rch)
}

func (f *futureImpl[R]) Fail(cause error) {
	f.rch <- newFailedAsyncResult[R](cause)
	close(f.rch)
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
	rch     chan Result[R]
	handler ResultHandler[R]
}

func (run futureRunner[R]) Run(ctx context.Context) {
	select {
	case <-ctx.Done():
		run.handler(ctx, *(new(R)), ctx.Err())
		return
	case ar, ok := <-run.rch:
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
