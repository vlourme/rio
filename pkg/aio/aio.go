package aio

import (
	"context"
	"errors"
	"net"
)

var (
	ErrUnexpectedCompletion      = errors.New("aio: unexpected completion error")
	ErrOperationDeadlineExceeded = errors.Join(errors.New("aio: operation deadline exceeded"), context.DeadlineExceeded)
	ErrEmptyBytes                = errors.New("aio: empty bytes")
	ErrNilAddr                   = errors.New("aio: addr is nil")
	ErrBusy                      = errors.New("aio: busy")
	ErrClosed                    = errors.Join(errors.New("aio: use of closed network connection"), net.ErrClosed)
)

func IsUnexpectedCompletionError(err error) bool {
	return errors.Is(err, ErrUnexpectedCompletion)
}

func IsOperationDeadlineExceededError(err error) bool {
	return errors.Is(err, ErrOperationDeadlineExceeded)
}

func IsEmptyBytesError(err error) bool {
	return errors.Is(err, ErrEmptyBytes)
}

func IsBusyError(err error) bool {
	return errors.Is(err, ErrBusy)
}

func IsClosedError(err error) bool {
	return errors.Is(err, ErrClosed)
}
