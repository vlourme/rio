package iouring

import (
	"syscall"
	"unsafe"
)

const (
	EnterGetEvents uint32 = 1 << iota
	EnterSQWakeup
	EnterSQWait
	EnterExtArg
	EnterRegisteredRing
)

/*
 * io_uring_enter - https://manpages.debian.org/unstable/liburing-dev/io_uring_enter.2.en.html
 */
func (ring *Ring) Enter(submitted uint32, waitNr uint32, flags uint32, sig unsafe.Pointer) (uint, error) {
	return ring.Enter2(submitted, waitNr, flags, sig, nSig/szDivider)
}

/*
 * io_uring_enter2 - https://manpages.debian.org/unstable/liburing-dev/io_uring_enter.2.en.html
 */
func (ring *Ring) Enter2(
	submitted uint32,
	waitNr uint32,
	flags uint32,
	sig unsafe.Pointer,
	size int,
) (uint, error) {
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

const (
	kernMaxEntries   = 32768
	kernMaxCQEntries = 2 * kernMaxEntries
)

const cqEntriesMultiplier = 2

// liburing: get_sq_cq_entries
func getSqCqEntries(entries uint32, p *Params, sq, cq *uint32) error {
	var cqEntries uint32

	if entries == 0 {
		return syscall.EINVAL
	}
	if entries > kernMaxEntries {
		if p.flags&SetupClamp == 0 {
			return syscall.EINVAL
		}
		entries = kernMaxEntries
	}

	entries = roundupPow2(entries)
	if p.flags&SetupCQSize != 0 {
		if p.cqEntries == 0 {
			return syscall.EINVAL
		}
		cqEntries = p.cqEntries
		if cqEntries > kernMaxCQEntries {
			if p.flags&SetupClamp == 0 {
				return syscall.EINVAL
			}
			cqEntries = kernMaxCQEntries
		}
		cqEntries = roundupPow2(cqEntries)
		if cqEntries < entries {
			return syscall.EINVAL
		}
	} else {
		cqEntries = cqEntriesMultiplier * entries
	}
	*sq = entries
	*cq = cqEntries

	return nil
}
