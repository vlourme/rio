package aio

import "syscall"

func fcntl(fd int, cmd int, arg int) (int, error) {
	val, _, errno := syscall.Syscall6(syscall.SYS_FCNTL, uintptr(fd), uintptr(cmd), uintptr(arg), 0, 0, 0)
	if val == -1 {
		return int(val), errno
	}
	return int(val), nil
}
