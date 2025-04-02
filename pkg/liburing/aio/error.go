package aio

import (
	"context"
	"errors"
	"syscall"
)

var (
	ErrCanceled  = &CanceledError{}
	ErrTimeout   = &TimeoutError{}
	ErrOpInvalid = errors.New("invalid operation")
)

func IsCanceled(err error) bool {
	return errors.Is(err, ErrCanceled)
}

func IsTimeout(err error) bool {
	return errors.Is(err, ErrTimeout) || errors.Is(err, context.DeadlineExceeded)
}

func IsOperationInvalid(err error) bool {
	return errors.Is(err, ErrOpInvalid)
}

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

func NewRingErr(err error) error {
	return &RingError{err}
}

type RingError struct {
	Err error
}

func (e *RingError) Error() string   { return "create iouring failed: " + e.Err.Error() }
func (e *RingError) Timeout() bool   { return false }
func (e *RingError) Temporary() bool { return false }
func (e *RingError) Is(err error) bool {
	return errors.Is(err, e.Err)
}

func NewInvalidOpErr(err error) error {
	return &OperationInvalidError{err}
}

type OperationInvalidError struct {
	Err error
}

func (e *OperationInvalidError) Error() string   { return "invalid operation: " + e.Err.Error() }
func (e *OperationInvalidError) Timeout() bool   { return false }
func (e *OperationInvalidError) Temporary() bool { return false }
func (e *OperationInvalidError) Is(err error) bool {
	return errors.Is(err, ErrOpInvalid)
}
