//go:build linux

package liburing

import (
	"errors"
	"sync/atomic"
	"unsafe"
)

func NewBufferAndRingConfig(ring *Ring, bgid uint16, entries uint16, flags uint32, buffer []byte) (config *BufferAndRingConfig, err error) {
	bLen := len(buffer)
	if bLen == 0 {
		err = errors.New("buffer is empty")
		return
	}
	if entries == 0 || int(entries) > bLen || bLen%int(entries) != 0 {
		err = errors.New("invalid entries")
		return
	}

	br, brErr := ring.SetupBufRing(uint32(entries), bgid, flags)
	if brErr != nil {
		err = brErr
		return
	}
	br.BufRingInit()

	size := bLen / int(entries)
	for i := uint16(0); i < entries; i++ {
		addr := unsafe.Pointer(&buffer[int(i)*size : int(i+1)*size][0])
		mask := BufferRingMask(uint32(entries))
		br.BufRingAdd(uintptr(addr), uint32(size), i, mask, int(i))
	}
	br.BufRingAdvance(int(entries))

	config = &BufferAndRingConfig{
		ring:    ring,
		entries: entries,
		bgid:    bgid,
		br:      br,
	}
	return
}

type BufferAndRingConfig struct {
	ring    *Ring
	entries uint16
	bgid    uint16
	br      *BufferAndRing
}

func (config *BufferAndRingConfig) Bid(cqe *CompletionQueueEvent) (bid uint16) {
	bid = uint16(cqe.Flags >> IORING_CQE_BUFFER_SHIFT)
	return
}

func (config *BufferAndRingConfig) Advance(count int) {
	config.br.BufRingAdvance(count)
}

func (config *BufferAndRingConfig) Close() (err error) {
	// munmap
	ringSizeAddr := uintptr(config.entries) * unsafe.Sizeof(BufferAndRing{})
	_ = munmap(uintptr(unsafe.Pointer(config.br)), ringSizeAddr)
	// unregister
	_, err = config.ring.UnregisterBufferRing(int(config.bgid))
	return
}

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

type BufReg struct {
	RingAddr    uint64
	RingEntries uint32
	Bgid        uint16
	Pad         uint16
	Resv        [3]uint64
}
