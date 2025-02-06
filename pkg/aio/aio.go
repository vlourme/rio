package aio

import (
	"errors"
	"net"
)

var (
	ErrUnexpectedCompletion = errors.New("aio: unexpected completion error")
	ErrBusy                 = errors.New("aio: busy")
	ErrClosed               = errors.Join(errors.New("aio: use of closed network connection"), net.ErrClosed)
)

func IsUnexpectedCompletionError(err error) bool {
	return errors.Is(err, ErrUnexpectedCompletion)
}

func IsBusy(err error) bool {
	return errors.Is(err, ErrBusy)
}
