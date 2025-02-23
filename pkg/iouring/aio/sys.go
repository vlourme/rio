package aio

import "syscall"

func fcntl(fd int, cmd int, arg int) (int, error) {
	r1, _, e1 := syscall.Syscall6(syscall.SYS_FCNTL, uintptr(fd), uintptr(cmd), uintptr(arg), 0, 0, 0)
	val := int32(r1)

	if val == -1 {
		return int(val), e1
	}
	return int(val), nil
}
