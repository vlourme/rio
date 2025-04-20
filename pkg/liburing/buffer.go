//go:build linux

package liburing

import (
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"
)

func (ring *Ring) SetupBufRing(entries uint32, bgid uint16, flags uint32) (*BufferAndRing, error) {
	br, err := ring.bufAndRingSetup(entries, bgid, flags)
	if br != nil {
		br.BufRingInit()
	}
	return br, err
}

func (ring *Ring) bufAndRingSetup(entries uint32, bgid uint16, flags uint32) (*BufferAndRing, error) {
	var br *BufferAndRing
	var reg *BufReg
	var ringSizeAddr uintptr
	var brPtr unsafe.Pointer
	var err error

	ringSizeAddr = uintptr(entries) * unsafe.Sizeof(BufferAndRing{})
	brPtr, err = mmap(0, ringSizeAddr, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_ANONYMOUS|syscall.MAP_PRIVATE, -1, 0)
	if err != nil {
		return nil, err
	}
	br = (*BufferAndRing)(brPtr)

	reg = &BufReg{}
	reg.RingAddr = uint64(uintptr(unsafe.Pointer(br)))
	reg.RingEntries = entries
	reg.Bgid = bgid
	reg.Flags = uint16(flags)

	_, err = ring.RegisterBufferRing(reg)
	if err != nil {
		_ = munmap(uintptr(unsafe.Pointer(br)), ringSizeAddr)
		return nil, err
	}
	runtime.KeepAlive(reg)
	return br, nil
}

func (ring *Ring) FreeBufRing(br *BufferAndRing, entries uint32, bgid uint16) (err error) {
	_, err = ring.UnregisterBufferRing(bgid)
	ringSizeAddr := uintptr(entries) * unsafe.Sizeof(BufferAndRing{})
	_ = munmap(uintptr(unsafe.Pointer(br)), ringSizeAddr)
	return
}

var bufferAndRingStructSize = uint16(unsafe.Sizeof(BufferAndRing{}))

type BufferAndRing struct {
	Addr uint64
	Len  uint32
	Bid  uint16
	Tail uint16
}

func (br *BufferAndRing) BufRingAdd(addr unsafe.Pointer, length uint32, bid uint16, mask, bufOffset uint16) {
	buf := (*BufferAndRing)(
		unsafe.Pointer(uintptr(unsafe.Pointer(br)) +
			(uintptr(((br.Tail + bufOffset) & mask) * bufferAndRingStructSize))))
	buf.Addr = uint64(uintptr(addr))
	buf.Len = length
	buf.Bid = bid
}

const bit16offset = 16

func (br *BufferAndRing) BufRingAdvance(count uint16) {
	newTail := br.Tail + count
	bidAndTail := (*uint32)(unsafe.Pointer(&br.Bid))
	bidAndTailVal := uint32(newTail)<<bit16offset + uint32(br.Bid)
	atomic.StoreUint32(bidAndTail, bidAndTailVal)
}

func (ring *Ring) internalBufRingCQAdvance(br *BufferAndRing, bufCount, cqeCount int) {
	br.BufRingAdvance(uint16(bufCount))
	ring.CQAdvance(uint32(cqeCount))
}

func (ring *Ring) BufRingCQAdvance(br *BufferAndRing, count int) {
	// note: it does not work well for [IORING_RECVSEND_BUNDLE]
	ring.internalBufRingCQAdvance(br, count, count)
}

func (br *BufferAndRing) BufRingInit() {
	br.Tail = 0
}

func BufferRingMask(entries uint32) uint32 {
	return entries - 1
}

type BufReg struct {
	RingAddr    uint64
	RingEntries uint32
	Bgid        uint16
	Flags       uint16
	Resv        [3]uint64
}
