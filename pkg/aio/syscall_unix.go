//go:build unix

package aio

import (
	"syscall"
)

func mmap(addr uintptr, length uintptr, prot int, flags int, fd int, offset int64) (xaddr uintptr, err error) {
	r0, _, e1 := syscall.Syscall6(syscall.SYS_MMAP, addr, length, uintptr(prot), uintptr(flags), uintptr(fd), uintptr(offset))
	xaddr = r0
	if e1 != 0 {
		err = e1
		return
	}
	return
}

func munmap(addr uintptr, length uintptr) (err error) {
	_, _, e1 := syscall.Syscall(syscall.SYS_MUNMAP, addr, length, 0)
	if e1 != 0 {
		err = e1
		return
	}
	return nil
}

func madvise(address, length, advice uintptr) (err error) {
	_, _, e1 := syscall.Syscall(syscall.SYS_MADVISE, address, length, advice)
	if e1 != 0 {
		err = e1
		return
	}
	return
}
