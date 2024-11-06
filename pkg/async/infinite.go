package async

import (
	"context"
	"runtime"
	"time"
)

func TryInfinitePromise[T any](ctx context.Context) (promise Promise[T], ok bool) {
	exec := From(ctx)
	submitter, has := exec.GetExecutorSubmitter()
	if has {
		promise = newInfinitePromise[T](ctx, submitter)
		ok = true
	}
	return
}

func MustInfinitePromise[T any](ctx context.Context) (promise Promise[T], err error) {
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

func newInfinitePromise[R any](ctx context.Context, submitter ExecutorSubmitter) Promise[R] {
	return newFuture[R](ctx, true, submitter)
}
