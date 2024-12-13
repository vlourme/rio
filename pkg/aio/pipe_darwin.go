//go:build darwin

package aio

import (
	"os"
	"syscall"
)

func Pipe2(p []int) error {
	if err := syscall.Pipe(p); err != nil {
		return os.NewSyscallError("pipe", err)
	}
	syscall.CloseOnExec(p[0])
	syscall.CloseOnExec(p[1])
	if err := syscall.SetNonblock(p[0], true); err != nil {
		_ = syscall.Close(p[0])
		_ = syscall.Close(p[1])
		return os.NewSyscallError("setnonblock", err)
	}
	if err := syscall.SetNonblock(p[1], true); err != nil {
		_ = syscall.Close(p[0])
		_ = syscall.Close(p[1])
		return os.NewSyscallError("setnonblock", err)
	}
	return nil
}
