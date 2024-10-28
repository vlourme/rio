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

func TryGetPromise[T any](ctx context.Context) (promise Promise[T], ok bool) {
	exec := From(ctx)
	if exec.Available() {
		ch, has := exec.GetExecutorSubmitter()
		if has {
			promise = newPromise[T](ctx, ch)
			ok = true
		}
	}
	return
}

func GetPromise[T any](ctx context.Context) (promise Promise[T], err error) {
	times := 10
	ok := false
	for {
		promise, ok = TryGetPromise[T](ctx)
		if ok {
			break
		}
		deadline, hasDeadline := ctx.Deadline()
		if hasDeadline && deadline.Before(time.Now()) {
			err = ErrGetPromiseFailedByTimeout
			break
		}
		times--
		if times < 0 {
			times = 10
			runtime.Gosched()
		}
	}
	return
}

func newPromise[R any](ctx context.Context, submitter ExecutorSubmitter) Promise[R] {
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	return &futureImpl[R]{
		ctx:       ctx,
		cancel:    cancel,
		rch:       make(chan Result[R], 1),
		submitter: submitter,
	}
}
