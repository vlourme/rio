//go:build windows

package aio

import "syscall"

func Cancel(op *Operator) {
	handle := syscall.Handle(op.fd.Fd())
	_ = syscall.CancelIoEx(handle, &op.overlapped)
}
