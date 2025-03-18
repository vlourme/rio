package aio

import (
	"context"
	"errors"
	"syscall"
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

// MapErr maps from the context errors to the historical internal net
// error values.
func MapErr(err error) error {
	switch err {
	case context.Canceled:
		return ErrCanceled
	case context.DeadlineExceeded:
		return ErrTimeout
	default:
		return err
	}
}

type CanceledError struct{}

func (e *CanceledError) Error() string   { return "operation was canceled" }
func (e *CanceledError) Timeout() bool   { return false }
func (e *CanceledError) Temporary() bool { return true }
func (e *CanceledError) Is(err error) bool {
	if errors.Is(err, context.Canceled) {
		return true
	}
	if errors.Is(err, syscall.ECANCELED) {
		return true
	}
	return false
}

type TimeoutError struct{}

func (e *TimeoutError) Error() string   { return "i/o timeout" }
func (e *TimeoutError) Timeout() bool   { return true }
func (e *TimeoutError) Temporary() bool { return true }
func (e *TimeoutError) Is(err error) bool {
	return err == context.DeadlineExceeded
}
