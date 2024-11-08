package async

import (
	"context"
	"runtime"
	"time"
)

type Promise[R any] interface {
	Complete(result R, err error)
	Succeed(result R)
	Fail(cause error)
	Cancel()
	SetDeadline(t time.Time)
	Future() (future Future[R])
}

func TryPromise[T any](ctx context.Context) (promise Promise[T], ok bool) {
	exec := From(ctx)
	submitter, has := exec.GetExecutorSubmitter()
	if has {
		promise = newPromise[T](ctx, submitter)
		ok = true
	}
	return
}

func MustPromise[T any](ctx context.Context) (promise Promise[T], err error) {
	times := 10
	ok := false
	for {
		promise, ok = TryPromise[T](ctx)
		if ok {
			break
		}
		if err = ctx.Err(); err != nil {
			break
		}
		time.Sleep(ns500)
		times--
		if times < 0 {
			times = 10
			runtime.Gosched()
		}
	}
	return
}

func newPromise[R any](ctx context.Context, submitter ExecutorSubmitter) Promise[R] {
	return newFuture[R](ctx, submitter, 1, false)
}
