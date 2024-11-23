package iouring

import (
	"os"
	"syscall"
	"unsafe"
)

/* FIXME */
// todo get hugePageSize from os
// cat /proc/meminfo | grep Huge
// cat /proc/meminfo | grep Hugepagesize
const hugePageSize uint64 = 2 * 1024 * 1024

// liburing: io_uring_alloc_huge
func allocHuge(
	entries uint32, p *Params, sq *SubmissionQueue, cq *CompletionQueue, buf unsafe.Pointer, bufSize uint64,
) (uint, error) {
	pageSize := uint64(os.Getpagesize())
	var sqEntries, cqEntries uint32
	var ringMem, sqesMem uint64
	var memUsed uint64
	var ptr unsafe.Pointer

	errno := getSqCqEntries(entries, p, &sqEntries, &cqEntries)
	if errno != nil {
		return 0, errno
	}

	sqesMem = uint64(sqEntries) * uint64(unsafe.Sizeof(SubmissionQueue{}))
	sqesMem = (sqesMem + pageSize - 1) &^ (pageSize - 1)
	ringMem = uint64(cqEntries) * uint64(unsafe.Sizeof(CompletionQueue{}))
	if p.flags&SetupCQE32 != 0 {
		ringMem *= 2
	}
	ringMem += uint64(sqEntries) * uint64(unsafe.Sizeof(uint32(0)))
	memUsed = sqesMem + ringMem
	memUsed = (memUsed + pageSize - 1) &^ (pageSize - 1)

	if buf == nil && (sqesMem > hugePageSize || ringMem > hugePageSize) {
		return 0, syscall.ENOMEM
	}

	if buf != nil {
		if memUsed > bufSize {
			return 0, syscall.ENOMEM
		}
		ptr = buf
	} else {
		var mapHugetlb int
		if sqesMem <= pageSize {
			bufSize = pageSize
		} else {
			bufSize = hugePageSize
			mapHugetlb = syscall.MAP_HUGETLB
		}
		var err error
		ptr, err = sysMmap(
			0, uintptr(bufSize),
			syscall.PROT_READ|syscall.PROT_WRITE,
			syscall.MAP_SHARED|syscall.MAP_ANONYMOUS|mapHugetlb, -1, 0)
		if err != nil {
			return 0, err
		}
	}

	sq.sqes = (*SubmissionQueueEntry)(ptr)
	if memUsed <= bufSize {
		sq.ringPtr = unsafe.Pointer(uintptr(unsafe.Pointer(sq.sqes)) + uintptr(sqesMem))
		cq.ringSize = 0
		sq.ringSize = 0
	} else {
		var mapHugetlb int
		if ringMem <= pageSize {
			bufSize = pageSize
		} else {
			bufSize = hugePageSize
			mapHugetlb = syscall.MAP_HUGETLB
		}
		var err error
		ptr, err = sysMmap(
			0, uintptr(bufSize),
			syscall.PROT_READ|syscall.PROT_WRITE,
			syscall.MAP_SHARED|syscall.MAP_ANONYMOUS|mapHugetlb, -1, 0)
		if err != nil {
			_ = sysMunmap(uintptr(unsafe.Pointer(sq.sqes)), 1)

			return 0, err
		}
		sq.ringPtr = ptr
		sq.ringSize = uint(bufSize)
		cq.ringSize = 0
	}

	cq.ringPtr = sq.ringPtr
	p.sqOff.userAddr = uint64(uintptr(unsafe.Pointer(sq.sqes)))
	p.cqOff.userAddr = uint64(uintptr(sq.ringPtr))

	return uint(memUsed), nil
}
