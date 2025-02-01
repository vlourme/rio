package aio

import (
	"errors"
	"net"
)

var (
	ErrUnexpectedCompletion      = errors.New("aio: unexpected completion error")
	ErrOperationDeadlineExceeded = errors.New("aio: operation deadline exceeded")
	ErrNilAddr                   = errors.New("aio: addr is nil")
	ErrBusy                      = errors.New("aio: busy")
	ErrClosed                    = errors.Join(errors.New("aio: use of closed network connection"), net.ErrClosed)
)

func IsUnexpectedCompletionError(err error) bool {
	return errors.Is(err, ErrUnexpectedCompletion)
}

func IsBusy(err error) bool {
	return errors.Is(err, ErrBusy)
}
