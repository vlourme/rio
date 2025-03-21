//go:build linux

package sys

import (
	"errors"
	"os"
	"sync/atomic"
	"syscall"
	"unsafe"
)

var (
	errEAGAIN error = syscall.EAGAIN
	errEINVAL error = syscall.EINVAL
	errENOENT error = syscall.ENOENT
)

var _zero uintptr

func errnoErr(e syscall.Errno) error {
	switch e {
	case 0:
		return nil
	case syscall.EAGAIN:
		return errEAGAIN
	case syscall.EINVAL:
		return errEINVAL
	case syscall.ENOENT:
		return errENOENT
	}
	return e
}

func IgnoringEINTRIO(fn func(fd int, p []byte) (int, error), fd int, p []byte) (int, error) {
	for {
		n, err := fn(fd, p)
		if err == nil {
			return n, nil
		}
		if !errors.Is(err, syscall.EINTR) {
			return n, err
		}
	}
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

func GetRLimit() (soft uint64, hard uint64, err error) {
	var r syscall.Rlimit
	if err = syscall.Getrlimit(syscall.RLIMIT_NOFILE, &r); err != nil {
		err = os.NewSyscallError("getrlimit", err)
		return
	}
	soft = r.Cur
	hard = r.Max
	return
}

func IsCloseOnExec(fd int) bool {
	r, err := fcntl(int32(fd), syscall.F_GETFD, syscall.FD_CLOEXEC)
	if err < 0 {
		return false
	}
	return r == 1
}

func Mmap(fd int, offset int64, length int, prot int, flags int) (b []byte, err error) {
	if length <= 0 {
		return nil, syscall.EINVAL
	}

	r0, _, e1 := syscall.Syscall6(syscall.SYS_MMAP, uintptr(0), uintptr(length), uintptr(prot), uintptr(flags), uintptr(fd), uintptr(offset))
	if e1 != 0 {
		err = errnoErr(e1)
		return
	}
	b = unsafe.Slice((*byte)(unsafe.Pointer(r0)), length)
	return
}

func Munmap(b []byte) (err error) {
	if len(b) == 0 || len(b) != cap(b) {
		return syscall.EINVAL
	}
	addr := uintptr(unsafe.Pointer(&b[0]))
	_, _, e1 := syscall.Syscall(syscall.SYS_MUNMAP, addr, uintptr(len(b)), 0)
	if e1 != 0 {
		err = errnoErr(e1)
	}
	return
}

func Madvise(b []byte, advice int) (err error) {
	var _p0 unsafe.Pointer
	if len(b) > 0 {
		_p0 = unsafe.Pointer(&b[0])
	} else {
		_p0 = unsafe.Pointer(&_zero)
	}
	_, _, e1 := syscall.Syscall(syscall.SYS_MADVISE, uintptr(_p0), uintptr(len(b)), uintptr(advice))
	if e1 != 0 {
		err = errnoErr(e1)
	}
	return
}
