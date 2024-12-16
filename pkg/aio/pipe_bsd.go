//go:build dragonfly || freebsd || netbsd || openbsd || linux

package aio

import (
	"os"
	"syscall"
)

func Pipe2(p []int) error {
	if err := syscall.Pipe2(p[:], syscall.O_NONBLOCK|syscall.O_CLOEXEC); err != nil {
		return os.NewSyscallError("pipe2", err)
	}
	return nil
}
