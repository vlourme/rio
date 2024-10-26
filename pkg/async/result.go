package async

type Result[R any] interface {
	Succeed() (succeed bool)
	Failed() (failed bool)
	Result() (result R)
	Cause() (err error)
}

func newAsyncResult[R any](result R, cause error) Result[R] {
	return &resultImpl[R]{
		result: result,
		cause:  cause,
	}
}

type resultImpl[R any] struct {
	result R
	cause  error
}

func (ar *resultImpl[R]) Succeed() (succeed bool) {
	succeed = ar.cause == nil
	return
}

func (ar *resultImpl[R]) Failed() (failed bool) {
	failed = ar.cause != nil
	return
}

func (ar *resultImpl[R]) Result() (result R) {
	result = ar.result
	return
}

func (ar *resultImpl[R]) Cause() (err error) {
	err = ar.cause
	return
}

type ResultHandler[R any] func(result R, err error)
