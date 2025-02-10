package rio

import (
	"github.com/brickingsoft/errors"
	"github.com/brickingsoft/rio/pkg/aio"
	"github.com/brickingsoft/rxp/async"
	"io"
	"time"
)

const (
	errMetaPkgKey = "pkg"
	errMetaPkgVal = "rio"
)

var (
	ErrEmptyBytes       = errors.Define("empty bytes", errors.WithMeta(errMetaPkgKey, errMetaPkgVal))
	ErrNetworkUnmatched = errors.Define("network is not matched", errors.WithMeta(errMetaPkgKey, errMetaPkgVal))
	ErrNilAddr          = errors.Define("addr is nil", errors.WithMeta(errMetaPkgKey, errMetaPkgVal))
	ErrAllocateBytes    = errors.Define("allocate bytes failed", errors.WithMeta(errMetaPkgKey, errMetaPkgVal))
	ErrClosed           = errors.Define("use a closed connection", errors.WithMeta(errMetaPkgKey, errMetaPkgVal))
)

var (
	ErrRead     = errors.Define("read failed", errors.WithMeta(errMetaPkgKey, errMetaPkgVal))
	ErrWrite    = errors.Define("write failed", errors.WithMeta(errMetaPkgKey, errMetaPkgVal))
	ErrReadFrom = errors.Define("read from failed", errors.WithMeta(errMetaPkgKey, errMetaPkgVal))
	ErrWriteTo  = errors.Define("write to failed", errors.WithMeta(errMetaPkgKey, errMetaPkgVal))
	ErrReadMsg  = errors.Define("read msg failed", errors.WithMeta(errMetaPkgKey, errMetaPkgVal))
	ErrWriteMsg = errors.Define("write msg failed", errors.WithMeta(errMetaPkgKey, errMetaPkgVal))
	ErrSendfile = errors.Define("sendfile failed", errors.WithMeta(errMetaPkgKey, errMetaPkgVal))
	ErrClose    = errors.Define("close failed", errors.WithMeta(errMetaPkgKey, errMetaPkgVal))
	ErrAccept   = errors.Define("accept failed", errors.WithMeta(errMetaPkgKey, errMetaPkgVal))
	ErrDial     = errors.Define("dial failed", errors.WithMeta(errMetaPkgKey, errMetaPkgVal))
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

func IsErrAllocateBytes(err error) bool {
	return errors.Is(err, ErrAllocateBytes)
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
