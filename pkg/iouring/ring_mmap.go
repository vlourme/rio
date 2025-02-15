package iouring

import (
	"syscall"
	"unsafe"
)

func setupRingPointers(p *Params, sq *SubmissionQueue, cq *CompletionQueue) {
	sq.head = (*uint32)(unsafe.Pointer(uintptr(sq.ringPtr) + uintptr(p.sqOff.head)))
	sq.tail = (*uint32)(unsafe.Pointer(uintptr(sq.ringPtr) + uintptr(p.sqOff.tail)))
	sq.ringMask = (*uint32)(unsafe.Pointer(uintptr(sq.ringPtr) + uintptr(p.sqOff.ringMask)))
	sq.ringEntries = (*uint32)(unsafe.Pointer(uintptr(sq.ringPtr) + uintptr(p.sqOff.ringEntries)))
	sq.flags = (*uint32)(unsafe.Pointer(uintptr(sq.ringPtr) + uintptr(p.sqOff.flags)))
	sq.dropped = (*uint32)(unsafe.Pointer(uintptr(sq.ringPtr) + uintptr(p.sqOff.dropped)))
	sq.array = (*uint32)(unsafe.Pointer(uintptr(sq.ringPtr) + uintptr(p.sqOff.array)))

	cq.head = (*uint32)(unsafe.Pointer(uintptr(cq.ringPtr) + uintptr(p.cqOff.head)))
	cq.tail = (*uint32)(unsafe.Pointer(uintptr(cq.ringPtr) + uintptr(p.cqOff.tail)))
	cq.ringMask = (*uint32)(unsafe.Pointer(uintptr(cq.ringPtr) + uintptr(p.cqOff.ringMask)))
	cq.ringEntries = (*uint32)(unsafe.Pointer(uintptr(cq.ringPtr) + uintptr(p.cqOff.ringEntries)))
	cq.overflow = (*uint32)(unsafe.Pointer(uintptr(cq.ringPtr) + uintptr(p.cqOff.overflow)))
	cq.cqes = (*CompletionQueueEvent)(unsafe.Pointer(uintptr(cq.ringPtr) + uintptr(p.cqOff.cqes)))
	if p.cqOff.flags != 0 {
		cq.flags = (*uint32)(unsafe.Pointer(uintptr(cq.ringPtr) + uintptr(p.cqOff.flags)))
	}
}

const (
	offsqRing    uint64 = 0
	offcqRing    uint64 = 0x8000000
	offSQEs      uint64 = 0x10000000
	offPbufRing  uint64 = 0x80000000
	offPbufShift uint64 = 16
	offMmapMask  uint64 = 0xf8000000
)

func MmapRing(fd int, p *Params, sq *SubmissionQueue, cq *CompletionQueue) error {
	var size uintptr
	var err error

	size = unsafe.Sizeof(CompletionQueueEvent{})
	if p.flags&SetupCQE32 != 0 {
		size += unsafe.Sizeof(CompletionQueueEvent{})
	}

	sq.ringSize = uint(uintptr(p.sqOff.array) + uintptr(p.sqEntries)*unsafe.Sizeof(uint32(0)))
	cq.ringSize = uint(uintptr(p.cqOff.cqes) + uintptr(p.cqEntries)*size)

	if p.features&FeatSingleMMap != 0 {
		if cq.ringSize > sq.ringSize {
			sq.ringSize = cq.ringSize
		}
		cq.ringSize = sq.ringSize
	}

	var ringPtr unsafe.Pointer
	ringPtr, err = mmap(0, uintptr(sq.ringSize), syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED|syscall.MAP_POPULATE, fd,
		int64(offsqRing))
	if err != nil {
		return err
	}
	sq.ringPtr = unsafe.Pointer(ringPtr)

	if p.features&FeatSingleMMap != 0 {
		cq.ringPtr = sq.ringPtr
	} else {
		ringPtr, err = mmap(0, uintptr(cq.ringSize), syscall.PROT_READ|syscall.PROT_WRITE,
			syscall.MAP_SHARED|syscall.MAP_POPULATE, fd,
			int64(offcqRing))
		if err != nil {
			cq.ringPtr = nil

			goto err
		}
		cq.ringPtr = ringPtr
	}

	size = unsafe.Sizeof(SubmissionQueueEntry{})
	if p.flags&SetupSQE128 != 0 {
		size += 64
	}
	ringPtr, err = mmap(0, size*uintptr(p.sqEntries), syscall.PROT_READ|syscall.PROT_WRITE,
		syscall.MAP_SHARED|syscall.MAP_POPULATE, fd, int64(offSQEs))
	if err != nil {
		goto err
	}
	sq.sqes = (*SubmissionQueueEntry)(ringPtr)
	setupRingPointers(p, sq, cq)
	return nil

err:
	UnmapRings(sq, cq)
	return err
}

func UnmapRings(sq *SubmissionQueue, cq *CompletionQueue) {
	if sq.ringSize > 0 {
		_ = munmap(uintptr(sq.ringPtr), uintptr(sq.ringSize))
	}
	if uintptr(cq.ringPtr) != 0 && cq.ringSize > 0 && cq.ringPtr != sq.ringPtr {
		_ = munmap(uintptr(cq.ringPtr), uintptr(cq.ringSize))
	}
}
