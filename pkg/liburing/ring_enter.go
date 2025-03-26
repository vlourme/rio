//go:build linux

package liburing

import (
	"syscall"
	"unsafe"
)

const (
	IORING_ENTER_GETEVENTS uint32 = 1 << iota
	IORING_ENTER_SQ_WAKEUP
	IORING_ENTER_SQ_WAIT
	IORING_ENTER_EXT_ARG
	IORING_ENTER_REGISTERED_RING
	IORING_ENTER_ABS_TIMER
	IORING_ENTER_EXT_ARG_REG
)

const (
	sysEnter  = 426
	nSig      = 65
	szDivider = 8
)

func (ring *Ring) Enter(submitted uint32, waitNr uint32, flags uint32, sig unsafe.Pointer) (uint, error) {
	return ring.Enter2(submitted, waitNr, flags, sig, nSig/szDivider)
}

func (ring *Ring) Enter2(submitted uint32, waitNr uint32, flags uint32, sig unsafe.Pointer, size int) (uint, error) {
	var (
		consumed uintptr
		errno    syscall.Errno
	)

	consumed, _, errno = syscall.Syscall6(
		sysEnter,
		uintptr(ring.enterRingFd),
		uintptr(submitted),
		uintptr(waitNr),
		uintptr(flags),
		uintptr(sig),
		uintptr(size),
	)
	if errno > 0 {
		return 0, errno
	}
	return uint(consumed), nil
}
