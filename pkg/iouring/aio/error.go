package aio

import (
	"context"
	"errors"
)

var (
	ErrCanceled      = &CanceledError{}
	ErrTimeout       = &TimeoutError{}
	ErrUnsupportedOp = errors.New("unsupported op")
)

func IsCanceled(err error) bool {
	return errors.Is(err, ErrCanceled)
}

func IsTimeout(err error) bool {
	return errors.Is(err, ErrTimeout) || errors.Is(err, context.DeadlineExceeded)
}

func IsUnsupported(err error) bool {
	return errors.Is(err, ErrUnsupportedOp)
}

type CanceledError struct{}

func (e *CanceledError) Error() string   { return "i/o canceled" }
func (e *CanceledError) Timeout() bool   { return false }
func (e *CanceledError) Temporary() bool { return true }
func (e *CanceledError) Is(err error) bool {
	return err == context.Canceled
}

type TimeoutError struct{}

func (e *TimeoutError) Error() string   { return "i/o timeout" }
func (e *TimeoutError) Timeout() bool   { return true }
func (e *TimeoutError) Temporary() bool { return true }
func (e *TimeoutError) Is(err error) bool {
	return err == context.DeadlineExceeded
}
