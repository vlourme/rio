package aio

import "syscall"

func Unlink(path string) error {
	return syscall.Unlink(path)
}
