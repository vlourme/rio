package async

import (
	"context"
	"io"
	"reflect"
)

type Result[R any] interface {
	Succeed() (succeed bool)
	Failed() (failed bool)
	Result() (result R)
	Cause() (err error)
}

func newResult[R any](result R, cause error) Result[R] {
	return resultImpl[R]{
		result: result,
		cause:  cause,
	}
}

func newSucceedResult[R any](result R) Result[R] {
	return resultImpl[R]{
		result: result,
		cause:  nil,
	}
}

func newFailedResult[R any](cause error) Result[R] {
	return resultImpl[R]{
		cause: cause,
	}
}

type resultImpl[R any] struct {
	result R
	cause  error
}

func (ar resultImpl[R]) Succeed() (succeed bool) {
	succeed = ar.cause == nil
	return
}

func (ar resultImpl[R]) Failed() (failed bool) {
	failed = ar.cause != nil
	return
}

func (ar resultImpl[R]) Result() (result R) {
	result = ar.result
	return
}

func (ar resultImpl[R]) Cause() (err error) {
	err = ar.cause
	return
}

type ResultHandler[R any] func(ctx context.Context, result R, err error)

func tryCloseResultWhenUnexpectedlyErrorOccur[R any](ar Result[R]) {
	if ar.Succeed() {
		r := ar.Result()
		ri := reflect.ValueOf(r).Interface()
		closer, isCloser := ri.(io.Closer)
		if isCloser {
			_ = closer.Close()
		}
	}
}

type Void struct{}
