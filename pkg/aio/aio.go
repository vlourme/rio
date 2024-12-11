package aio

import (
	"errors"
	"os"
)

var (
	ErrUnexpectedCompletion      = errors.New("aio: unexpected completion error")
	ErrOperationDeadlineExceeded = errors.Join(errors.New("aio: operation deadline exceeded"), os.ErrDeadlineExceeded)
	ErrEmptyBytes                = errors.New("aio: empty bytes")
	ErrBusy                      = errors.New("aio: busy")
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
