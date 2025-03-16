package aio

import (
	"context"
	"errors"
)

var (
	ErrUncompleted   = errors.New("uncompleted")
	ErrTimeout       = &TimeoutError{}
	ErrUnsupportedOp = errors.New("unsupported op")
)

type TimeoutError struct{}

func (e *TimeoutError) Error() string   { return "i/o timeout" }
func (e *TimeoutError) Timeout() bool   { return true }
func (e *TimeoutError) Temporary() bool { return true }

func (e *TimeoutError) Is(err error) bool {
	return err == context.DeadlineExceeded
}

func IsUncompleted(err error) bool {
	return errors.Is(err, ErrUncompleted)
}

func IsTimeout(err error) bool {
	return errors.Is(err, ErrTimeout) || errors.Is(err, context.DeadlineExceeded)
}

func IsUnsupported(err error) bool {
	return errors.Is(err, ErrUnsupportedOp)
}
