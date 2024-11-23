package iouring

import (
	"sync/atomic"
	"syscall"
	"unsafe"
)

const (
	CQEFBuffer uint32 = 1 << iota
	CQEFMore
	CQEFSockNonempty
	CQEFNotif
)

const CQEBufferShift uint32 = 16

// liburing: io_uring_cq
type CompletionQueue struct {
	head        *uint32
	tail        *uint32
	ringMask    *uint32
	ringEntries *uint32
	flags       *uint32
	overflow    *uint32
	cqes        *CompletionQueueEvent

	ringSize uint
	ringPtr  unsafe.Pointer

	// nolint: unused
	pad [2]uint32
}

// liburing: io_uring_cqe
type CompletionQueueEvent struct {
	UserData uint64
	Res      int32
	Flags    uint32

	// FIXME
	// 	__u64 big_cqe[];
}

// liburing: io_uring_cqe_get_data - https://manpages.debian.org/unstable/liburing-dev/io_uring_cqe_get_data.3.en.html
func (c *CompletionQueueEvent) GetData() unsafe.Pointer {
	return unsafe.Pointer(uintptr(c.UserData))
}

// liburing: io_uring_cqe_get_data64 - https://manpages.debian.org/unstable/liburing-dev/io_uring_cqe_get_data64.3.en.html
func (c *CompletionQueueEvent) GetData64() uint64 {
	return c.UserData
}

const CQEventFdDisabled uint32 = 1 << 0

// liburing: io_cqring_offsets
type CQRingOffsets struct {
	head        uint32
	tail        uint32
	ringMask    uint32
	ringEntries uint32
	overflow    uint32
	cqes        uint32
	flags       uint32
	resv1       uint32
	userAddr    uint64
}

// liburing: __io_uring_peek_cqe
func internalPeekCQE(ring *Ring, nrAvailable *uint32) (*CompletionQueueEvent, error) {
	var cqe *CompletionQueueEvent
	var err error
	var available uint32
	var shift uint32
	mask := *ring.cqRing.ringMask

	if ring.flags&SetupCQE32 != 0 {
		shift = 1
	}

	for {
		tail := atomic.LoadUint32(ring.cqRing.tail)
		head := *ring.cqRing.head

		cqe = nil
		available = tail - head
		if available == 0 {
			break
		}

		cqe = (*CompletionQueueEvent)(
			unsafe.Add(unsafe.Pointer(ring.cqRing.cqes), uintptr((head&mask)<<shift)*unsafe.Sizeof(CompletionQueueEvent{})),
		)

		if ring.features&FeatExtArg == 0 && cqe.UserData == liburingUdataTimeout {
			if cqe.Res < 0 {
				err = syscall.Errno(uintptr(-cqe.Res))
			}
			ring.CQAdvance(1)
			if err == nil {
				continue
			}
			cqe = nil
		}

		break
	}

	if nrAvailable != nil {
		*nrAvailable = available
	}

	return cqe, err
}
