package async

import (
	"context"
	"errors"
	"time"
)

var (
	ErrFutureWasClosed = errors.New("rio: promise was closed")
)

// Future
// 许诺的未来，注册一个异步非堵塞的结果处理器。
type Future[R any] interface {
	// OnComplete
	// 注册一个结果处理器，它是异步非堵塞的。
	// 除了 Promise.Fail 给到的错误外，还有可以有以下错误。
	// context.Canceled 已取消
	// context.DeadlineExceeded 已超时
	// ErrFutureWasClosed 非无限流许诺的不正常关闭
	OnComplete(handler ResultHandler[R])
}

type Awaitable[R any] interface {
	Await() (v R, err error)
}

// Await
// 同步等待未来结果。
// 注意，非无限流许诺只有一个未来，而无限流许诺可能有多个未来。
// 对于无限流许诺，直到 err 不为空时才算结束。
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

func newFuture[R any](ctx context.Context, submitter ExecutorSubmitter, buf int, stream bool) *futureImpl[R] {
	futureCtx, futureCtxCancel := context.WithCancel(ctx)
	if buf < 1 {
		buf = 1
	}
	return &futureImpl[R]{
		ctx:                     ctx,
		futureCtx:               futureCtx,
		futureCtxCancel:         futureCtxCancel,
		futureDeadlineCtxCancel: nil,
		stream:                  stream,
		rch:                     newResultChan[R](buf),
		submitter:               submitter,
	}
}

type futureImpl[R any] struct {
	ctx                     context.Context
	futureCtx               context.Context
	futureCtxCancel         context.CancelFunc
	futureDeadlineCtxCancel context.CancelFunc
	stream                  bool
	rch                     *resultChan[R]
	submitter               ExecutorSubmitter
}

func (f *futureImpl[R]) OnComplete(handler ResultHandler[R]) {
	run := futureRunner[R]{
		ctx:            f.futureCtx,
		cancel:         f.futureCtxCancel,
		deadlineCancel: f.futureDeadlineCtxCancel,
		stream:         f.stream,
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
		stream:         f.stream,
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
	if !f.stream {
		f.rch.Close()
	}
}

func (f *futureImpl[R]) Succeed(result R) {
	f.rch.Emit(newSucceedAsyncResult[R](result))
	if !f.stream {
		f.rch.Close()
	}
}

func (f *futureImpl[R]) Fail(cause error) {
	f.rch.Emit(newFailedAsyncResult[R](cause))
	if !f.stream {
		f.rch.Close()
	}
}

func (f *futureImpl[R]) Cancel() {
	f.futureCtxCancel()
}

func (f *futureImpl[R]) SetDeadline(t time.Time) {
	f.futureCtx, f.futureDeadlineCtxCancel = context.WithDeadline(f.futureCtx, t)
}

func (f *futureImpl[R]) Future() (future Future[R]) {
	future = f
	return
}

type futureRunner[R any] struct {
	ctx            context.Context
	cancel         context.CancelFunc
	deadlineCancel context.CancelFunc
	stream         bool
	rch            *resultChan[R]
	handler        ResultHandler[R]
}

func (run futureRunner[R]) Run(ctx context.Context) {
	futureCtx := run.ctx
	futureCtxCancel := run.cancel
	rch := run.rch
	stopped := false
	isUnexpectedError := false
	for {
		select {
		case <-ctx.Done():
			run.handler(ctx, *(new(R)), ctx.Err())
			stopped = true
			isUnexpectedError = true
			break
		case <-futureCtx.Done():
			run.handler(ctx, *(new(R)), futureCtx.Err())
			stopped = true
			isUnexpectedError = true
			break
		case ar, ok := <-rch.ch:
			if !ok {
				if !run.stream {
					run.handler(ctx, *(new(R)), ErrFutureWasClosed)
				}
				stopped = true
				break
			}
			if ar.Succeed() {
				run.handler(ctx, ar.Result(), nil)
			} else {
				run.handler(ctx, *(new(R)), ar.Cause())
			}
			if !run.stream {
				stopped = true
			}
			break
		}
		if stopped {
			break
		}
	}
	if isUnexpectedError {
		rch.CloseUnexpectedly()
	}
	futureCtxCancel()
	if run.deadlineCancel != nil {
		run.deadlineCancel()
	}
}
