//go:build linux

package bytebuffers

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

func (buf *buffer) Close() (err error) {
	runtime.SetFinalizer(buf, nil)
	err = munmap(uintptr(unsafe.Pointer(&buf.b[0])), doubleSize(buf.c))
	if err != nil {
		err = fmt.Errorf("bytebuffers.Buffer: close failed, %v", err)
	}
	return
}

func (buf *buffer) grow(n int) (err error) {
	if n < 1 {
		return
	}
	defer func() {
		if ex := recover(); ex != nil {
			err = fmt.Errorf("bytebuffers.Buffer: grow failed, %v", ex)
		}
	}()

	if buf.b != nil {
		n = n - buf.r
		// left shift
		copy(buf.b, buf.b[buf.r:buf.w])
		buf.w -= buf.r
		buf.a = buf.w
		buf.r = 0
		if n < 1 { // has place for n
			return
		}
	}

	// has no more place
	adjustedSize := adjustBufferSize(n)

	nb, nbErr := allocateBuffer(adjustedSize)
	if nbErr != nil {
		err = nbErr
		return
	}
	copy(nb, buf.b[buf.r:buf.w])
	buf.b = nb
	buf.w = buf.w - buf.r
	buf.a = buf.w
	buf.r = 0

	buf.c += adjustedSize
	return
}

func doubleSize(size int) int {
	return size * 2
}

func allocateBuffer(size int) (b []byte, err error) {
	nofd := ^uintptr(0)

	vaddr, mmapErr := mmap(0, doubleSize(size), syscall.MAP_SHARED|syscall.MAP_ANONYMOUS, nofd)
	if mmapErr != nil {
		err = fmt.Errorf("bytebuffers.Buffer: mmap failed, %v", mmapErr)
		return
	}

	fd, fdCreateErr := unix.MemfdCreate("bytebuffer", 0)
	if fdCreateErr != nil {
		err = fmt.Errorf("bytebuffers.Buffer: failed to create memfd buffer, %v", fdCreateErr)
		return
	}

	ftruncateErr := unix.Ftruncate(fd, int64(size))
	if ftruncateErr != nil {
		err = fmt.Errorf("bytebuffers.Buffer: failed to ftruncate memfd buffer, %v", ftruncateErr)
		return
	}

	fdptr := uintptr(fd)

	_, mmap1Err := mmap(vaddr, size, syscall.MAP_SHARED|syscall.MAP_FIXED, fdptr)
	if mmap1Err != nil {
		err = fmt.Errorf("first mmap first fixed failed, %v", mmap1Err)
		return
	}

	_, mmap2Err := mmap(vaddr+uintptr(size), size, syscall.MAP_SHARED|syscall.MAP_FIXED, fdptr)
	if mmap2Err != nil {
		err = fmt.Errorf("first mmap second fixed failed, %v", mmap2Err)
		return
	}

	_ = syscall.Close(fd)

	bptr := (*byte)(unsafe.Pointer(vaddr))
	b = unsafe.Slice(bptr, doubleSize(size))

	return
}

func mmap(addr uintptr, length, flags int, fd uintptr) (uintptr, error) {
	result, _, err := syscall.Syscall6(
		syscall.SYS_MMAP,
		addr,
		uintptr(length),
		uintptr(syscall.PROT_READ|syscall.PROT_WRITE),
		uintptr(flags),
		fd,
		uintptr(0),
	)
	if err != 0 {
		return 0, os.NewSyscallError("mmap error", err)
	}

	return result, nil
}

func munmap(addr uintptr, length int) error {
	_, _, err := syscall.Syscall(
		syscall.SYS_MUNMAP,
		addr,
		uintptr(length),
		0,
	)
	if err != 0 {
		return os.NewSyscallError("munmap error", err)
	}

	return nil
}
