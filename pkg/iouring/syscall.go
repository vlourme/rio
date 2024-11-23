package iouring

import (
	"syscall"
	"unsafe"
)

func mmap(addr uintptr, length uintptr, prot int, flags int, fd int, offset int64) (xaddr uintptr, err error) {
	r0, _, e1 := syscall.Syscall6(syscall.SYS_MMAP, addr, length, uintptr(prot), uintptr(flags), uintptr(fd), uintptr(offset))
	xaddr = r0
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

func sysMmap(addr, length uintptr, prot, flags, fd int, offset int64) (unsafe.Pointer, error) {
	r0, _, e1 := syscall.Syscall6(syscall.SYS_MMAP, addr, length, uintptr(prot), uintptr(flags), uintptr(fd), uintptr(offset))
	//xaddr = r0
	if e1 != 0 {
		return unsafe.Pointer(nil), errnoErr(e1)
	}
	//ptr, err := mmap(addr, length, prot, flags, fd, offset)
	return unsafe.Pointer(r0), nil
}

func sysMunmap(addr, length uintptr) error {
	_, _, e1 := syscall.Syscall(syscall.SYS_MUNMAP, addr, length, 0)
	if e1 != 0 {
		return errnoErr(e1)
	}
	//return munmap(addr, length)
	return nil
}

func sysMadvise(address, length, advice uintptr) error {
	_, _, err := syscall.Syscall(syscall.SYS_MADVISE, address, length, advice)
	return err
}
