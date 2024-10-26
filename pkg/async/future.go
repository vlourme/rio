package async

import "errors"

var (
	ErrFutureClosed = errors.New("rio: future closed")
)

type Future[R any] interface {
	OnComplete(handler ResultHandler[R])
}

func SucceedFuture[R any](result R) Future[R] {
	ch := make(chan Result[R], 1)
	ch <- newAsyncResult[R](result, nil)
	return newFuture[R](ch)
}

func FailedFuture[R any](cause error) Future[R] {
	ch := make(chan Result[R], 1)
	ch <- newAsyncResult[R](nil, cause)
	return newFuture[R](ch)
}

func newFuture[R any](ch <-chan Result[R]) Future[R] {
	return &futureImpl[R]{ch: ch}
}

type futureImpl[R any] struct {
	ch <-chan Result[R]
}

func (f *futureImpl[R]) OnComplete(handler ResultHandler[R]) {
	ar, ok := <-f.ch
	if !ok {
		handler(nil, ErrFutureClosed)
		return
	}
	handler(ar.Result(), ar.Cause())
}
