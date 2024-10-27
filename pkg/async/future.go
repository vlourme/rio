package async

import "errors"

var (
	ErrFutureClosed = errors.New("rio: future closed")
)

type Promise[R any] interface {
	Complete(result R, err error)
	Succeed(result R)
	Fail(cause error)
	Future() (future Future[R])
}

type Future[R any] interface {
	OnComplete(handler ResultHandler[R])
}

func SucceedFuture[R any](result R) Future[R] {
	ch := make(chan Result[R], 1)
	ch <- newSucceedAsyncResult[R](result)
	return newFuture[R](ch)
}

func FailedFuture[R any](cause error) Future[R] {
	ch := make(chan Result[R], 1)
	ch <- newFailedAsyncResult[R](cause)
	return newFuture[R](ch)
}

func New[R any]() Promise[R] {
	return &futureImpl[R]{
		ch: make(chan Result[R], 1),
	}
}

func newFuture[R any](ch chan Result[R]) Future[R] {
	return &futureImpl[R]{ch: ch}
}

type futureImpl[R any] struct {
	ch      chan Result[R]
	handler ResultHandler[R]
}

func (f *futureImpl[R]) OnComplete(handler ResultHandler[R]) {
	f.setHandler(handler)
}

func (f *futureImpl[R]) Complete(result R, err error) {
	if err == nil {
		f.Succeed(result)
	} else {
		f.Fail(err)
	}
}

func (f *futureImpl[R]) Succeed(result R) {
	f.ch <- newSucceedAsyncResult[R](result)
	close(f.ch)
}

func (f *futureImpl[R]) Fail(cause error) {
	f.ch <- newFailedAsyncResult[R](cause)
	close(f.ch)
}

func (f *futureImpl[R]) Future() (future Future[R]) {
	future = f
	return
}

func (f *futureImpl[R]) setHandler(handler ResultHandler[R]) {
	f.handler = handler
	defaultRunnable.Run(f.await)
}

func (f *futureImpl[R]) await() {
	ar, ok := <-f.ch
	if !ok {
		f.handler(*new(R), ErrFutureClosed)
		return
	}
	f.handler(ar.Result(), ar.Cause())
}
