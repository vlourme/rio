//go:build linux

package liburing

import (
	"sync/atomic"
	"unsafe"
)

func (ring *Ring) GetSQE() *SubmissionQueueEntry {
	sq := ring.sqRing
	var head, next uint32
	var shift int

	if ring.flags&SetupSQE128 != 0 {
		shift = 1
	}
	head = atomic.LoadUint32(sq.head)
	next = sq.sqeTail + 1
	if next-head <= *sq.ringEntries {
		sqe := (*SubmissionQueueEntry)(
			unsafe.Add(unsafe.Pointer(ring.sqRing.sqes),
				uintptr((sq.sqeTail&*sq.ringMask)<<shift)*unsafe.Sizeof(SubmissionQueueEntry{})),
		)
		sq.sqeTail = next
		return sqe
	}
	return nil
}

func (ring *Ring) SQEntries() uint32 {
	return *ring.sqRing.ringEntries
}

func (ring *Ring) SQReady() uint32 {
	khead := *ring.sqRing.head
	if ring.flags&SetupSQPoll != 0 {
		khead = atomic.LoadUint32(ring.sqRing.head)
	}
	return ring.sqRing.sqeTail - khead
}

func (ring *Ring) SQSpaceLeft() uint32 {
	return *ring.sqRing.ringEntries - ring.SQReady()
}

func (ring *Ring) SQRingWait() (uint, error) {
	if ring.flags&SetupSQPoll == 0 {
		return 0, nil
	}
	if ring.SQSpaceLeft() != 0 {
		return 0, nil
	}
	return ring.sqRingWait()
}

func (ring *Ring) sqRingNeedsEnter(submit uint32, flags *uint32) bool {
	if submit == 0 {
		return false
	}
	if (ring.flags & SetupSQPoll) == 0 {
		return true
	}
	if atomic.LoadUint32(ring.sqRing.flags)&SQNeedWakeup != 0 {
		*flags |= EnterSQWakeup
		return true
	}
	return false
}
