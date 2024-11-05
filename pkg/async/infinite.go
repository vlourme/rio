package async

import (
	"context"
	"runtime"
	"time"
)

type InfinitePromise[R any] interface {
	Complete(result R, err error)
	Succeed(result R)
	Fail(cause error)
	Cancel()
	Future() (future Future[R])
	Close()
}

func TryInfinitePromise[T any](ctx context.Context) (promise InfinitePromise[T], ok bool) {
	exec := From(ctx)
	submitter, has := exec.GetExecutorSubmitter()
	if has {
		promise = newInfinitePromise[T](ctx, submitter)
		ok = true
	}
	return
}

func MustInfinitePromise[T any](ctx context.Context) (promise InfinitePromise[T], err error) {
	times := 10
	ok := false
	for {
		promise, ok = TryInfinitePromise[T](ctx)
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

func newInfinitePromise[R any](ctx context.Context, submitter ExecutorSubmitter) InfinitePromise[R] {
	return newFuture[R](ctx, true, submitter)
}
