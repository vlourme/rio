package async

import (
	"context"
	"runtime"
	"time"
)

// TryInfinitePromise
// 尝试获取一个无限流的许诺，如果资源已耗光则获取不到。
// 无限流的特性是可以无限次完成许诺，而不是一次。
// 但要注意，必须在不需要它后，调用 Promise.Cancel 来关闭它。
func TryInfinitePromise[T any](ctx context.Context, buf int) (promise Promise[T], ok bool) {
	exec := From(ctx)
	submitter, has := exec.GetExecutorSubmitter()
	if has {
		promise = newInfinitePromise[T](ctx, submitter, buf)
		ok = true
	}
	return
}

// MustInfinitePromise
// 必须获得一个无限许诺，如果资源已耗光则等待，直到可以或者上下文错误。
func MustInfinitePromise[T any](ctx context.Context, buf int) (promise Promise[T], err error) {
	times := 10
	ok := false
	for {
		promise, ok = TryInfinitePromise[T](ctx, buf)
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

func newInfinitePromise[R any](ctx context.Context, submitter ExecutorSubmitter, buf int) Promise[R] {
	return newFuture[R](ctx, submitter, buf, true)
}
