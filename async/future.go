package async

import (
	"context"
	"errors"
	"sync"
	"time"
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
		err = errors.New("async: future is not a Awaitable[E]")
		return
	}
	v, err = awaitable.Await()
	return
}

func SucceedImmediately[R any](ctx context.Context, r R) (f Future[R]) {
	f = &immediatelyFuture[R]{
		ctx:   ctx,
		r:     r,
		cause: nil,
	}
	return
}

func FailedImmediately[R any](ctx context.Context, cause error) (f Future[R]) {
	f = &immediatelyFuture[R]{
		ctx:   ctx,
		r:     *(new(R)),
		cause: cause,
	}
	return
}

type immediatelyFuture[R any] struct {
	ctx   context.Context
	r     R
	cause error
}

func (f *immediatelyFuture[R]) OnComplete(handler ResultHandler[R]) {
	handler(f.ctx, f.r, f.cause)
	return
}

func (f *immediatelyFuture[R]) Await() (r R, err error) {
	r, err = f.r, f.cause
	return
}

func newFuture[R any](ctx context.Context, submitter TaskSubmitter, buf int, stream bool) *futureImpl[R] {
	if buf < 1 {
		buf = 1
	}
	var streamLocker sync.Locker
	if stream {
		streamLocker = new(spinLock)
	}
	return &futureImpl[R]{
		ctx:             ctx,
		futureCtx:       ctx,
		futureCtxCancel: nil,
		stream:          stream,
		streamClosed:    false,
		streamLocker:    streamLocker,
		rch:             make(chan result[R], buf),
		submitter:       submitter,
	}
}

type futureImpl[R any] struct {
	ctx             context.Context
	futureCtx       context.Context
	futureCtxCancel context.CancelFunc
	stream          bool
	streamClosed    bool
	streamLocker    sync.Locker
	rch             chan result[R]
	submitter       TaskSubmitter
	handler         ResultHandler[R]
}

func (f *futureImpl[R]) handle() {
	futureCtx := f.futureCtx
	rch := f.rch
	stopped := false
	isUnexpectedError := false
	for {
		select {
		case <-f.ctx.Done():
			f.handler(f.ctx, *(new(R)), f.ctx.Err())
			stopped = true
			isUnexpectedError = true
			break
		case <-futureCtx.Done():
			f.handler(f.ctx, *(new(R)), futureCtx.Err())
			stopped = true
			isUnexpectedError = true
			break
		case ar, ok := <-rch:
			if !ok {
				f.handler(f.ctx, *(new(R)), context.Canceled)
				stopped = true
				break
			}
			f.handler(f.ctx, ar.entry, ar.cause)
			if !f.stream {
				stopped = true
			}
			break
		}
		if stopped {
			break
		}
	}
	if isUnexpectedError {
		for {
			ar, ok := <-rch
			if !ok {
				break
			}
			tryCloseResultWhenUnexpectedlyErrorOccur(ar)
		}
	}
	if f.futureCtxCancel != nil {
		f.futureCtxCancel()
	}
	f.clean()
}

func (f *futureImpl[R]) clean() {
	f.futureCtx = nil
	f.futureCtxCancel = nil
	f.rch = nil
	f.submitter = nil
	f.handler = nil
}

func (f *futureImpl[R]) OnComplete(handler ResultHandler[R]) {
	f.handler = handler
	f.submitter.Submit(f.handle)
}

func (f *futureImpl[R]) Await() (v R, err error) {
	ch := make(chan result[R], 1)
	var handler ResultHandler[R] = func(ctx context.Context, r R, cause error) {
		ch <- result[R]{
			entry: r,
			cause: cause,
		}
		close(ch)
	}
	f.handler = handler
	f.submitter.Submit(f.handle)
	ar := <-ch
	v = ar.entry
	err = ar.cause
	return
}

func (f *futureImpl[R]) Complete(r R, err error) {
	if f.stream {
		f.streamLocker.Lock()
		if f.streamClosed {
			tryCloseResultWhenUnexpectedlyErrorOccur(result[R]{
				entry: r,
				cause: err,
			})
			f.streamLocker.Unlock()
			return
		}
	}
	f.rch <- result[R]{
		entry: r,
		cause: err,
	}
	if f.stream {
		f.streamLocker.Unlock()
	} else {
		close(f.rch)
	}
}

func (f *futureImpl[R]) Succeed(r R) {
	if f.stream {
		f.streamLocker.Lock()
		if f.streamClosed {
			tryCloseResultWhenUnexpectedlyErrorOccur(result[R]{
				entry: r,
				cause: nil,
			})
			f.streamLocker.Unlock()
			return
		}
	}
	f.rch <- result[R]{
		entry: r,
		cause: nil,
	}
	if f.stream {
		f.streamLocker.Unlock()
	} else {
		close(f.rch)
	}
}

func (f *futureImpl[R]) Fail(cause error) {
	if f.stream {
		f.streamLocker.Lock()
		if f.streamClosed {
			f.streamLocker.Unlock()
			return
		}
	}
	f.rch <- result[R]{
		cause: cause,
	}
	if f.stream {
		f.streamLocker.Unlock()
	} else {
		close(f.rch)
	}
}

func (f *futureImpl[R]) Cancel() {
	if f.stream {
		f.streamLocker.Lock()
		if f.streamClosed {
			f.streamLocker.Unlock()
			return
		}
	}
	if f.stream {
		f.streamClosed = true
		f.streamLocker.Unlock()
	}
	if f.futureCtxCancel != nil {
		f.futureCtxCancel()
	}
	close(f.rch)
}

func (f *futureImpl[R]) SetDeadline(deadline time.Time) {
	f.futureCtx, f.futureCtxCancel = context.WithDeadline(f.futureCtx, deadline)
}

func (f *futureImpl[R]) Future() (future Future[R]) {
	future = f
	return
}
