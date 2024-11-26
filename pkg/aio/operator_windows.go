//go:build windows

package aio

import (
	"syscall"
	"time"
)

type Operator struct {
	overlapped syscall.Overlapped
	fd         Fd
	userdata   Userdata
	callback   OperationCallback
	completion OperatorCompletion
	timeout    time.Duration
	timer      *operatorTimer
}

type operatorCanceler struct {
	handle     syscall.Handle
	overlapped *syscall.Overlapped
}

func (op *operatorCanceler) Cancel() {
	_ = syscall.CancelIoEx(op.handle, op.overlapped)
}
