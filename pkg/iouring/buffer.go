package iouring

import (
	"sync/atomic"
	"unsafe"
)

// liburing: io_uring_buf
// liburing: io_uring_buf_ring
type BufAndRing struct {
	Addr uint64
	Len  uint32
	Bid  uint16
	Tail uint16
}

const PbufRingMMap = 1

// liburing: io_uring_buf_reg
type BufReg struct {
	RingAddr    uint64
	RingEntries uint32
	Bgid        uint16
	Pad         uint16
	Resv        [3]uint64
}

var RingBufStructSize = uint16(unsafe.Sizeof(BufAndRing{}))

// liburing: io_uring_buf_ring_add - https://manpages.debian.org/unstable/liburing-dev/io_uring_buf_ring_add.3.en.html
func (br *BufAndRing) BufRingAdd(addr uintptr, length uint32, bid uint16, mask, bufOffset int) {
	buf := (*BufAndRing)(
		unsafe.Pointer(uintptr(unsafe.Pointer(br)) +
			(uintptr(((br.Tail + uint16(bufOffset)) & uint16(mask)) * RingBufStructSize))))
	buf.Addr = uint64(addr)
	buf.Len = length
	buf.Bid = bid
}

const bit16offset = 16

// liburing: io_uring_buf_ring_advance - https://manpages.debian.org/unstable/liburing-dev/io_uring_buf_ring_advance.3.en.html
func (br *BufAndRing) BufRingAdvance(count int) {
	newTail := br.Tail + uint16(count)
	// FIXME: implement 16 bit version of atomic.Store
	bidAndTail := (*uint32)(unsafe.Pointer(&br.Bid))
	bidAndTailVal := uint32(newTail)<<bit16offset + uint32(br.Bid)
	atomic.StoreUint32(bidAndTail, bidAndTailVal)
}

// liburing:  __io_uring_buf_ring_cq_advance - https://manpages.debian.org/unstable/liburing-dev/__io_uring_buf_ring_cq_advance.3.en.html
func (ring *Ring) internalBufRingCQAdvance(br *BufAndRing, bufCount, cqeCount int) {
	br.Tail += uint16(bufCount)
	ring.CQAdvance(uint32(cqeCount))
}

// liburing: io_uring_buf_ring_cq_advance - https://manpages.debian.org/unstable/liburing-dev/io_uring_buf_ring_cq_advance.3.en.html
func (ring *Ring) BufRingCQAdvance(br *BufAndRing, count int) {
	ring.internalBufRingCQAdvance(br, count, count)
}

// liburing: io_uring_buf_ring_init - https://manpages.debian.org/unstable/liburing-dev/io_uring_buf_ring_init.3.en.html
func (br *BufAndRing) BufRingInit() {
	br.Tail = 0
}

// liburing: io_uring_buf_ring_mask - https://manpages.debian.org/unstable/liburing-dev/io_uring_buf_ring_mask.3.en.html
func BufRingMask(entries uint32) int {
	return int(entries - 1)
}
