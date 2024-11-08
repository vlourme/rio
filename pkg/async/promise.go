package async

import (
	"context"
	"runtime"
	"time"
)

// Promise
// 许诺一个未来。
type Promise[R any] interface {
	// Complete
	// 完成
	Complete(result R, err error)
	// Succeed
	// 成功完成
	Succeed(result R)
	// Fail
	// 错误完成
	Fail(cause error)
	// Cancel
	// 取消许诺，未来会是一个 context.Canceled 错误。
	Cancel()
	// SetDeadline
	// 设置死期。
	// 当超时后，未来会是一个 context.DeadlineExceeded 错误。
	SetDeadline(t time.Time)
	// Future
	// 返回未来
	Future() (future Future[R])
}

// TryPromise
// 尝试获取一个许诺，如果资源已耗光则获取不到。
// 许诺只能完成一次，完成后则不可再用。
// 当 Promise.Complete ，Promise.Succeed ，Promise.Fail 后，不必再 Promise.Cancel 来关闭它。
func TryPromise[T any](ctx context.Context) (promise Promise[T], ok bool) {
	exec := From(ctx)
	submitter, has := exec.GetExecutorSubmitter()
	if has {
		promise = newPromise[T](ctx, submitter)
		ok = true
	}
	return
}

// MustPromise
// 必须获得一个许诺，如果资源已耗光则等待，直到可以或者上下文错误。
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
