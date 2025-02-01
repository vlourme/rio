package rio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/aio"
	"github.com/brickingsoft/rxp/async"
	"io"
	"time"
)

var (
	ErrClosed           = errors.New("rio: use a closed connection")
	ErrEmptyBytes       = errors.New("rio: empty bytes")
	ErrNetworkUnmatched = errors.New("rio: network is not matched")
	ErrNilAddr          = errors.New("rio: addr is nil")
	ErrAllocate         = errors.New("rio: allocate bytes failed")
	ErrAllocateWritten  = errors.New("rio: allocate written failed")
	ErrDeadlineExceeded = errors.New("rio: deadline exceeded")
)

// IsClosed
// 是否为使用一个已关闭的链接错误
func IsClosed(err error) bool {
	return errors.Is(err, ErrClosed)
}

// IsShutdown
// 是否为服务停止错误
func IsShutdown(err error) bool {
	return async.IsExecutorsClosed(err)
}

func IsBusy(err error) bool {
	return async.IsBusy(err) || aio.IsBusy(err)
}

func IsErrEmptyBytes(err error) bool {
	return errors.Is(err, ErrEmptyBytes)
}

func IsErrNetworkUnmatched(err error) bool {
	return errors.Is(err, ErrNetworkUnmatched)
}

func IsErrAllocate(err error) bool {
	return errors.Is(err, ErrAllocate)
}

func IsErrAllocateWrote(err error) bool {
	return errors.Is(err, ErrAllocateWritten)
}

func IsDeadlineExceeded(err error) bool {
	return async.IsDeadlineExceeded(err)
}
func GetDeadlineFromErr(err error) (time.Time, bool) {
	deadlineErr, ok := async.AsDeadlineExceededError(err)
	if ok {
		return deadlineErr.Deadline, true
	}
	return time.Time{}, false
}

func IsUnexpectedCompletedError(err error) bool {
	return aio.IsUnexpectedCompletionError(err)
}

func IsUnexpectedContextFailed(err error) bool {
	return async.IsUnexpectedContextFailed(err)
}

func GetUnexpectedContextFailedErr(err error) error {
	ctxErr, ok := async.AsUnexpectedContextError(err)
	if ok {
		return ctxErr.CtxErr
	}
	return nil
}

func IsEOF(err error) bool {
	return errors.Is(err, io.EOF)
}
