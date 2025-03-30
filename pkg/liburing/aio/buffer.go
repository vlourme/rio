package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"sync"
	"unsafe"
)

func newRingBuffer(ring *liburing.Ring, bgid uint16, entries uint16, flags uint32, buffer []byte) (config *RingBuffer, err error) {
	bLen := len(buffer)
	if bLen == 0 {
		err = errors.New("rb is empty")
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
		mask := liburing.BufferRingMask(uint32(entries))
		br.BufRingAdd(uintptr(addr), uint32(size), i, mask, int(i))
	}
	br.BufRingAdvance(int(entries))

	config = &RingBuffer{
		ring:    ring,
		entries: entries,
		bgid:    bgid,
		size:    size,
		br:      br,
	}
	return
}

type RingBuffer struct {
	ring    *liburing.Ring
	entries uint16
	bgid    uint16
	size    int
	br      *liburing.BufferAndRing
	buffer  []byte
}

func (rb *RingBuffer) Bid(cqe *liburing.CompletionQueueEvent) (bid uint16) {
	bid = uint16(cqe.Flags >> liburing.IORING_CQE_BUFFER_SHIFT)
	return
}

func (rb *RingBuffer) ReadBid(bid int, n int, b []byte) int {
	beg := bid * rb.size
	end := beg + n
	return copy(b, rb.buffer[beg:end])
}

func (rb *RingBuffer) Advance(count int) {
	rb.br.BufRingAdvance(count)
}

func (rb *RingBuffer) Close() (err error) {
	// munmap
	ringSizeAddr := uintptr(rb.entries) * unsafe.Sizeof(liburing.BufferAndRing{})
	_ = sys.MunmapPtr(uintptr(unsafe.Pointer(rb.br)), ringSizeAddr)
	// unregister
	_, err = rb.ring.UnregisterBufferRing(int(rb.bgid))
	return
}

func newRingBufferConfig(ring *liburing.Ring, size uint32, count uint32) *RingBufferConfig {
	if size == 0 {
		size = 4096
	}
	if count == 0 {
		count = 16
	}

	config := &RingBufferConfig{
		ring:    ring,
		size:    int(size),
		count:   uint16(count),
		buffers: sync.Pool{},
	}

	config.buffers.New = func() interface{} {
		return make([]byte, config.size*int(config.count))
	}
	return config
}

type RingBufferConfig struct {
	ring    *liburing.Ring
	size    int
	count   uint16
	buffers sync.Pool
}

func (config *RingBufferConfig) AcquireRingBuffer(fd *Fd) (*RingBuffer, error) {
	bgid, _ := fd.FileDescriptor()

	b := config.buffers.Get().([]byte)
	rb, rbErr := newRingBuffer(config.ring, uint16(bgid), config.count, 0, b)

	if rbErr != nil {
		return nil, rbErr
	}
	return rb, nil
}

func (config *RingBufferConfig) ReleaseRingBuffer(buf *RingBuffer) (err error) {
	if buf == nil {
		return
	}
	b := buf.buffer
	buf.buffer = nil

	config.buffers.Put(b)

	err = buf.Close()
	return
}
