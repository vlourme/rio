//go:build linux

package sys

import (
	"sync/atomic"
	"syscall"
)

func boolint(b bool) int {
	if b {
		return 1
	}
	return 0
}

var dupCloexecUnsupported atomic.Bool

func DupCloseOnExec(fd int) (int, string, error) {
	if syscall.F_DUPFD_CLOEXEC != 0 && !dupCloexecUnsupported.Load() {
		r0, err := Fcntl(fd, syscall.F_DUPFD_CLOEXEC, 0)
		if err == nil {
			return r0, "", nil
		}
		switch err {
		case syscall.EINVAL, syscall.ENOSYS:
			// Old kernel, or js/wasm (which returns
			// ENOSYS). Fall back to the portable way from
			// now on.
			dupCloexecUnsupported.Store(true)
		default:
			return -1, "fcntl", err
		}
	}
	return dupCloseOnExecOld(fd)
}

func dupCloseOnExecOld(fd int) (int, string, error) {
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	newfd, err := syscall.Dup(fd)
	if err != nil {
		return -1, "dup", err
	}
	syscall.CloseOnExec(newfd)
	return newfd, "", nil
}

func Fcntl(fd int, cmd int, arg int) (int, error) {
	val, errno := fcntl(int32(fd), int32(cmd), int32(arg))
	if val == -1 {
		return int(val), syscall.Errno(errno)
	}
	return int(val), nil
}

func fcntl(fd, cmd, arg int32) (ret int32, errno int32) {
	r, _, err := syscall.Syscall6(syscall.SYS_FCNTL, uintptr(fd), uintptr(cmd), uintptr(arg), 0, 0, 0)
	return int32(r), int32(err)
}
