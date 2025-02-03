//go:build windows

package aio

import "syscall"

func Cancel(op *Operator) {
	if op.processing.Load() && op.fd != nil {
		handle := syscall.Handle(op.fd.Fd())
		overlapped := &op.overlapped
		_ = syscall.CancelIoEx(handle, overlapped)
	}
}
