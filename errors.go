package rio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/aio"
	"github.com/brickingsoft/rxp/async"
	"io"
	"net"
)

var (
	ErrClosed           = errors.New("rio: closed")
	ErrBusy             = errors.New("rio: system busy")
	ErrEmptyBytes       = errors.New("rio: empty bytes")
	ErrNetworkUnmatched = errors.New("rio: network is not matched")
	ErrNilAddr          = errors.New("rio: addr is nil")
	ErrAllocate         = errors.New("rio: allocate bytes failed")
	ErrAllocateWrote    = errors.New("rio: allocate wrote failed")
	ErrDeadlineExceeded = errors.Join(errors.New("rio: deadline exceeded"), aio.ErrOperationDeadlineExceeded)
)

// IsClosed
// 是否为服务停止错误
func IsClosed(err error) bool {
	var opErr *net.OpError
	isOpErr := errors.As(err, &opErr)
	if isOpErr {
		err = opErr.Err
	}
	ok := errors.Is(err, async.ExecutorsClosed)
	return ok
}

func IsBusy(err error) bool {
	var opErr *net.OpError
	isOpErr := errors.As(err, &opErr)
	if isOpErr {
		err = opErr.Err
	}
	return errors.Is(err, ErrBusy) || aio.IsBusyError(err)
}

func IsErrEmptyBytes(err error) bool {
	var opErr *net.OpError
	isOpErr := errors.As(err, &opErr)
	if isOpErr {
		err = opErr.Err
	}
	return errors.Is(err, ErrEmptyBytes) || aio.IsEmptyBytesError(err)
}

func IsErrNetworkUnmatched(err error) bool {
	var opErr *net.OpError
	isOpErr := errors.As(err, &opErr)
	if isOpErr {
		err = opErr.Err
	}
	return errors.Is(err, ErrNetworkUnmatched)
}

func IsErrAllocate(err error) bool {
	var opErr *net.OpError
	isOpErr := errors.As(err, &opErr)
	if isOpErr {
		err = opErr.Err
	}
	return errors.Is(err, ErrAllocate)
}

func IsErrAllocateWrote(err error) bool {
	var opErr *net.OpError
	isOpErr := errors.As(err, &opErr)
	if isOpErr {
		err = opErr.Err
	}
	return errors.Is(err, ErrAllocateWrote)
}

func IsDeadlineExceeded(err error) bool {
	var opErr *net.OpError
	isOpErr := errors.As(err, &opErr)
	if isOpErr {
		err = opErr.Unwrap()
	}
	return async.IsDeadlineExceeded(err) || aio.IsOperationDeadlineExceededError(err)
}

func IsUnexpectedCompletedError(err error) bool {
	var opErr *net.OpError
	isOpErr := errors.As(err, &opErr)
	if isOpErr {
		err = opErr.Err
	}
	return aio.IsUnexpectedCompletionError(err)
}

func IsEOF(err error) bool {
	return errors.Is(err, io.EOF)
}
