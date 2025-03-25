//go:build linux

package liburing

import (
	"syscall"
	"unsafe"
)

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

var (
	errEAGAIN error = syscall.EAGAIN
	errEINVAL error = syscall.EINVAL
	errENOENT error = syscall.ENOENT
)

func mmap(addr uintptr, length uintptr, prot int, flags int, fd int, offset int64) (ptr unsafe.Pointer, err error) {
	r1, _, e1 := syscall.Syscall6(syscall.SYS_MMAP, addr, length, uintptr(prot), uintptr(flags), uintptr(fd), uintptr(offset))
	ptr = unsafe.Pointer(r1)
	if e1 != 0 {
		err = errnoErr(e1)
	}
	return
}

func munmap(addr uintptr, length uintptr) (err error) {
	_, _, e1 := syscall.Syscall(syscall.SYS_MUNMAP, addr, length, 0)
	if e1 != 0 {
		return errnoErr(e1)
	}
	return nil
}

func madvise(address, length, advice uintptr) error {
	_, _, err := syscall.Syscall(syscall.SYS_MADVISE, address, length, advice)
	return err
}
