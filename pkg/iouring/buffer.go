//go:build linux

package iouring

import (
	"sync/atomic"
	"unsafe"
)

var bufferAndRingStructSize = uint16(unsafe.Sizeof(BufferAndRing{}))

type BufferAndRing struct {
	Addr uint64
	Len  uint32
	Bid  uint16
	Tail uint16
}

func (br *BufferAndRing) BufRingAdd(addr uintptr, length uint32, bid uint16, mask, bufOffset int) {
	buf := (*BufferAndRing)(
		unsafe.Pointer(uintptr(unsafe.Pointer(br)) +
			(uintptr(((br.Tail + uint16(bufOffset)) & uint16(mask)) * bufferAndRingStructSize))))
	buf.Addr = uint64(addr)
	buf.Len = length
	buf.Bid = bid
}

const bit16offset = 16

func (br *BufferAndRing) BufRingAdvance(count int) {
	newTail := br.Tail + uint16(count)
	bidAndTail := (*uint32)(unsafe.Pointer(&br.Bid))
	bidAndTailVal := uint32(newTail)<<bit16offset + uint32(br.Bid)
	atomic.StoreUint32(bidAndTail, bidAndTailVal)
}

func (ring *Ring) internalBufRingCQAdvance(br *BufferAndRing, bufCount, cqeCount int) {
	br.Tail += uint16(bufCount)
	ring.CQAdvance(uint32(cqeCount))
}

func (ring *Ring) BufRingCQAdvance(br *BufferAndRing, count int) {
	ring.internalBufRingCQAdvance(br, count, count)
}

func (br *BufferAndRing) BufRingInit() {
	br.Tail = 0
}

func BufferRingMask(entries uint32) int {
	return int(entries - 1)
}

const PbufRingMMap = 1

type BufReg struct {
	RingAddr    uint64
	RingEntries uint32
	Bgid        uint16
	Pad         uint16
	Resv        [3]uint64
}
