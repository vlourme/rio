package async

import (
	"context"
	"runtime"
	"time"
)

// TryStreamPromise
// 尝试获取一个无限流的许诺，如果资源已耗光则获取不到。
// 无限流的特性是可以无限次完成许诺，而不是一次。
// 但要注意，必须在不需要它后，调用 Promise.Cancel 来关闭它。
// 关于许诺值，如果它实现了 io.Closer ，则当 Promise.Cancel 后且它未没处理，那么会自动 转化为 io.Closer 进行关闭。
// 由于在关闭后依旧可以完成许诺，因此所许诺的内容如果含有关闭功能，则请实现 io.Closer。
func TryStreamPromise[T any](ctx context.Context, buf int) (promise Promise[T], ok bool) {
	exec := From(ctx)
	submitter, has := exec.GetExecutorSubmitter()
	if has {
		promise = newStreamPromise[T](ctx, submitter, buf)
		ok = true
	}
	return
}

// MustStreamPromise
// 必须获得一个无限许诺，如果资源已耗光则等待，直到可以或者上下文错误。
// 无限流的特性是可以无限次完成许诺，而不是一次。
// 但要注意，必须在不需要它后，调用 Promise.Cancel 来关闭它。
// 关于许诺值，如果它实现了 io.Closer ，则当 Promise.Cancel 后且它未没处理，那么会自动 转化为 io.Closer 进行关闭。
// 由于在关闭后依旧可以完成许诺，因此所许诺的内容如果含有关闭功能，则请实现 io.Closer。
func MustStreamPromise[T any](ctx context.Context, buf int) (promise Promise[T], err error) {
	times := 10
	ok := false
	for {
		promise, ok = TryStreamPromise[T](ctx, buf)
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

func newStreamPromise[R any](ctx context.Context, submitter ExecutorSubmitter, buf int) Promise[R] {
	return newFuture[R](ctx, submitter, buf, true)
}
