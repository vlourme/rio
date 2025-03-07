//go:build linux

package aio

import (
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

func fcntl(fd int, cmd int, arg int) (int, error) {
	r1, _, e1 := syscall.Syscall6(syscall.SYS_FCNTL, uintptr(fd), uintptr(cmd), uintptr(arg), 0, 0, 0)
	val := int32(r1)

	if val == -1 {
		return int(val), e1
	}
	return int(val), nil
}

func mmap(fd int, offset int64, length int, prot int, flags int) (b []byte, err error) {
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

func munmap(b []byte) (err error) {
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

func madvise(b []byte, advice int) (err error) {
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
