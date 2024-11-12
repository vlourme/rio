package async

import (
	"context"
	"io"
	"reflect"
)

type result[E any] struct {
	entry E
	cause error
}

type ResultHandler[E any] func(ctx context.Context, entry E, cause error)

func tryCloseResultWhenUnexpectedlyErrorOccur[R any](ar result[R]) {
	if ar.cause == nil {
		r := ar.entry
		ri := reflect.ValueOf(r).Interface()
		closer, isCloser := ri.(io.Closer)
		if isCloser {
			_ = closer.Close()
		}
	}
}

type Void struct{}
