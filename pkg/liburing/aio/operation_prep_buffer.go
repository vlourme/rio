//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/queue"
	"math"
	"sync"
	"syscall"
	"unsafe"
)

func newBufferConfig(enabled bool, size uint32, count uint32) *BufferConfig {
	if size == 0 {
		size = 4096
	}
	if count == 0 {
		count = 16
	}

	config := &BufferConfig{
		enabled:     enabled,
		size:        int(size),
		count:       int(count),
		bufferQueue: queue.New[Buffer](),
		buffers:     sync.Pool{},
		iovecs:      sync.Pool{},
	}

	config.buffers.New = func() interface{} {
		return make([]byte, config.size*config.count)
	}
	config.iovecs.New = func() interface{} {
		return make([]syscall.Iovec, config.count)
	}
	if enabled {
		for i := 0; i < math.MaxUint16; i++ {
			config.bufferQueue.Enqueue(&Buffer{
				bgid:   i,
				size:   config.size,
				b:      nil,
				iovecs: nil,
			})
		}
	}

	return config
}

type BufferConfig struct {
	enabled     bool
	size        int
	count       int
	bufferQueue *queue.Queue[Buffer]
	buffers     sync.Pool
	iovecs      sync.Pool
}

func (config *BufferConfig) AcquireBuffer() *Buffer {
	if !config.enabled {
		return nil
	}
	buf := config.bufferQueue.Dequeue()
	if buf == nil {
		return nil
	}
	b := config.buffers.Get().([]byte)
	iovecs := config.iovecs.Get().([]syscall.Iovec)
	buf.b = b
	buf.iovecs = iovecs
	for i := range buf.iovecs {
		buf.iovecs[i] = syscall.Iovec{
			Base: &buf.b[i*config.size],
			Len:  uint64(config.size),
		}
	}
	return buf
}

func (config *BufferConfig) ReleaseBuffer(buf *Buffer) {
	if config.enabled && buf != nil {
		b := buf.b
		buf.b = nil
		config.buffers.Put(b)
		iovecs := buf.iovecs
		buf.iovecs = nil
		config.iovecs.Put(iovecs)
		config.bufferQueue.Enqueue(buf)
	}
}

type Buffer struct {
	bgid   int
	size   int
	b      []byte
	iovecs []syscall.Iovec
}

func (buf *Buffer) Read(bid int, n int, b []byte) int {
	beg := bid * buf.size
	end := beg + n
	return copy(b, buf.b[beg:end])
}

func (op *Operation) PrepareProvideBuffers(bgid int, buffers []syscall.Iovec) (err error) {
	op.code = liburing.IORING_OP_PROVIDE_BUFFERS
	op.fd = bgid
	op.addr = unsafe.Pointer(&buffers[0])
	op.addrLen = uint32(len(buffers))
	return
}

func (op *Operation) packingProvideBuffers(sqe *liburing.SubmissionQueueEntry) (err error) {
	bgid := op.fd
	addr := uintptr(op.addr)
	addrLen := op.addrLen
	nr := int(addrLen)
	sqe.PrepareProvideBuffers(addr, addrLen, nr, bgid, 0)
	sqe.SetData(unsafe.Pointer(op))
	return
}

func (op *Operation) PrepareRemoveBuffers(bgid int, buffers []syscall.Iovec) (err error) {
	op.code = liburing.IORING_OP_REMOVE_BUFFERS
	op.fd = bgid
	op.addrLen = uint32(len(buffers))
	return
}

func (op *Operation) packingRemoveBuffers(sqe *liburing.SubmissionQueueEntry) (err error) {
	bgid := op.fd
	nr := int(op.addrLen)
	sqe.PrepareRemoveBuffers(nr, bgid)
	sqe.SetData(unsafe.Pointer(op))
	return
}
