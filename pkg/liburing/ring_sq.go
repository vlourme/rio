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

	if ring.flags&IORING_SETUP_SQE128 != 0 {
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
	if ring.flags&IORING_SETUP_SQPOLL != 0 {
		khead = atomic.LoadUint32(ring.sqRing.head)
	}
	return ring.sqRing.sqeTail - khead
}

func (ring *Ring) SQSpaceLeft() uint32 {
	return *ring.sqRing.ringEntries - ring.SQReady()
}

func (ring *Ring) SQRingWait() (uint, error) {
	if ring.flags&IORING_SETUP_SQPOLL == 0 {
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
	if ring.flags&IORING_SETUP_SQPOLL == 0 {
		return true
	}
	if atomic.LoadUint32(ring.sqRing.flags)&IORING_SQ_NEED_WAKEUP != 0 {
		*flags |= IORING_ENTER_SQ_WAKEUP
		return true
	}
	return false
}

func (ring *Ring) flushSQ() uint32 {
	sq := ring.sqRing
	tail := sq.sqeTail
	if sq.sqeHead != tail {
		sq.sqeHead = tail
		atomic.StoreUint32(sq.tail, tail)
	}
	return tail - atomic.LoadUint32(sq.head)
}

func (ring *Ring) FlushSQ() bool {
	if n := ring.flushSQ(); n > 0 {
		return ring.SQNeedWakeup()
	}
	return false
}

func (ring *Ring) SQNeedWakeup() bool {
	if ring.flags&IORING_SETUP_SQPOLL == 0 {
		return false
	}
	return atomic.LoadUint32(ring.sqRing.flags)&IORING_SQ_NEED_WAKEUP != 0
}

func (ring *Ring) WakeupSQPoll() (uint, error) {
	var ret uint
	var err error
	if ring.SQNeedWakeup() {
		flags := IORING_ENTER_SQ_WAKEUP
		if ring.kind&regRing != 0 {
			flags |= IORING_ENTER_REGISTERED_RING
		}
		ret, err = ring.Enter(1, 0, flags, nil)
		if err != nil {
			return 0, err
		}
	}
	return ret, nil
}
