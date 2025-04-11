package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"io"
	"math"
	"os"
	"sync"
	"time"
	"unsafe"
)

type BufferAndRingConfig struct {
	Size        int
	Count       int
	IdleTimeout time.Duration
}

type BufferAndRing struct {
	bgid        uint16
	lastUseTime time.Time
	config      BufferAndRingConfig
	value       *liburing.BufferAndRing
	buffer      []byte
}

func (br *BufferAndRing) Id() uint16 {
	return br.bgid
}

func (br *BufferAndRing) WriteTo(length int, cqeFlags uint32, writer io.Writer) (n int, err error) {
	if length == 0 {
		br.value.BufRingAdvance(1)
		return
	}

	var (
		bid  = cqeFlags >> liburing.IORING_CQE_BUFFER_SHIFT
		beg  = int(bid) * br.config.Size
		end  = beg + length
		bLen = len(br.buffer)
		nn   = 0
	)

	if remains := end - bLen; remains > 0 { // split
		for {
			nn, err = writer.Write(br.buffer[beg:])
			if err != nil {
				break
			}
			n += nn
			beg += nn
			if beg == bLen {
				break
			}
		}
		beg = 0
		end = remains
	}

	for {
		nn, err = writer.Write(br.buffer[beg:end])
		if err != nil {
			break
		}
		n += nn
		beg += nn
		if beg == end {
			break
		}
	}

	used := uint16(math.Ceil(float64(length) / float64(br.config.Size)))
	br.value.BufRingAdvance(used)

	return
}

func (br *BufferAndRing) free(ring *liburing.Ring) (err error) {
	entries := uint32(br.config.Count)
	err = ring.FreeBufRing(br.value, entries, br.bgid)
	return
}

const (
	maxBufferSize = int(^uint(0) >> 1)
)

func newBufferAndRings(config BufferAndRingConfig) (brs *BufferAndRings, err error) {
	size := config.Size
	if size < 1 {
		size = os.Getpagesize()
	}
	config.Size = size

	count := config.Count
	if count == 0 {
		count = 16
	}
	count = int(liburing.RoundupPow2(uint32(count)))
	if count > 32768 {
		err = errors.New("count is too large for buffer and ring, max count is 32768")
		return
	}
	config.Count = count

	bLen := size * count
	if bLen > maxBufferSize {
		err = errors.New("size and count are too large for buffer and ring")
		return
	}

	if config.IdleTimeout < 1 {
		config.IdleTimeout = 10 * time.Second
	}

	bgids := make([]uint16, math.MaxUint16)
	for i := 0; i < len(bgids); i++ {
		bgids[i] = uint16(i)
	}

	brs = &BufferAndRings{
		config:       config,
		locker:       new(sync.Mutex),
		eventLoop:    nil,
		wg:           &sync.WaitGroup{},
		done:         make(chan struct{}),
		bgids:        bgids,
		idles:        make([]*BufferAndRing, 0, 8),
		bufferLength: bLen,
		bufferPool:   sync.Pool{},
	}

	return
}

type BufferAndRings struct {
	config       BufferAndRingConfig
	locker       sync.Locker
	eventLoop    *EventLoop
	wg           *sync.WaitGroup
	done         chan struct{}
	bgids        []uint16
	idles        []*BufferAndRing
	bufferLength int
	bufferPool   sync.Pool
}

func (brs *BufferAndRings) getBuffer() []byte {
	v := brs.bufferPool.Get()
	if v == nil {
		return make([]byte, brs.bufferLength)
	}
	return v.([]byte)
}

func (brs *BufferAndRings) putBuffer(buf []byte) {
	if len(buf) == 0 {
		return
	}
	brs.bufferPool.Put(buf)
}

func (brs *BufferAndRings) Acquire() (br *BufferAndRing, err error) {
	brs.locker.Lock()

	// no idles
	if len(brs.idles) == 0 {
		br, err = brs.createBufferAndRing()
		brs.locker.Unlock()
		return
	}
	// use an idle one
	br = brs.idles[0]
	brs.idles = brs.idles[1:]

	brs.locker.Unlock()
	return
}

func (brs *BufferAndRings) Release(br *BufferAndRing) {
	if br == nil {
		return
	}

	brs.locker.Lock()
	br.lastUseTime = time.Now()
	brs.idles = append(brs.idles, br)
	brs.locker.Unlock()
	return
}

func (brs *BufferAndRings) createBufferAndRing() (value *BufferAndRing, err error) {
	if len(brs.bgids) == 0 {
		err = errors.New("create buffer and ring failed cause no bgid available")
		return
	}

	bgid := brs.bgids[0]

	entries := uint32(brs.config.Count)
	br, setupErr := brs.eventLoop.ring.SetupBufRing(entries, bgid, 0)
	if setupErr != nil {
		err = setupErr
		return
	}

	brs.bgids = brs.bgids[1:]

	mask := liburing.BufferRingMask(entries)
	buffer := brs.getBuffer()
	bufferUnitLength := uint32(brs.config.Size)
	for i := uint32(0); i < entries; i++ {
		beg := bufferUnitLength * i
		end := beg + bufferUnitLength
		slice := buffer[beg:end]
		addr := &slice[0]
		br.BufRingAdd(unsafe.Pointer(addr), bufferUnitLength, uint16(i), uint16(mask), uint16(i))
	}
	br.BufRingAdvance(uint16(entries))

	value = &BufferAndRing{
		bgid:        bgid,
		lastUseTime: time.Time{},
		config:      brs.config,
		value:       br,
		buffer:      buffer,
	}
	return
}

func (brs *BufferAndRings) closeBufferAndRing(br *BufferAndRing) {
	// submit
	op := brs.eventLoop.resource.AcquireOperation()
	op.flags |= op_f_noexec
	op.cmd = op_cmd_close_br
	op.addr = unsafe.Pointer(br)
	_, _, _ = brs.eventLoop.SubmitAndWait(op)
	brs.eventLoop.resource.ReleaseOperation(op)
	// recycle br
	brs.recycleBufferAndRing(br)
	return
}

func (brs *BufferAndRings) recycleBufferAndRing(br *BufferAndRing) {
	// release bgid
	brs.bgids = append(brs.bgids, br.bgid)
	// release buffer
	buffer := br.buffer
	br.buffer = nil
	brs.putBuffer(buffer)
	return
}

func (brs *BufferAndRings) Start(eventLoop *EventLoop) {
	brs.eventLoop = eventLoop
	brs.wg.Add(1)
	go func(brs *BufferAndRings) {
		defer brs.wg.Done()

		done := brs.done
		var scratch []*BufferAndRing
		maxIdleDuration := brs.config.IdleTimeout
		stopped := false
		timer := time.NewTimer(maxIdleDuration)
		for {
			select {
			case <-done:
				stopped = true
				break
			case <-timer.C:
				brs.clean(&scratch)
				timer.Reset(maxIdleDuration)
				break
			}
			if stopped {
				break
			}
		}
		timer.Stop()
	}(brs)
}

func (brs *BufferAndRings) Stop() {
	close(brs.done)
	brs.wg.Wait()
}

func (brs *BufferAndRings) clean(scratch *[]*BufferAndRing) {
	brs.locker.Lock()

	n := len(brs.idles)
	if n == 0 {
		brs.locker.Unlock()
		return
	}

	maxIdleDuration := brs.config.IdleTimeout
	criticalTime := time.Now().Add(-maxIdleDuration)

	l, r, mid := 0, n-1, 0
	for l <= r {
		mid = (l + r) / 2
		if criticalTime.After(brs.idles[mid].lastUseTime) {
			l = mid + 1
		} else {
			r = mid - 1
		}
	}
	i := r
	if i == -1 {
		brs.locker.Unlock()
		return
	}

	*scratch = append((*scratch)[:0], brs.idles[:i+1]...)
	m := copy(brs.idles, brs.idles[i+1:])
	for i = m; i < n; i++ {
		brs.idles[i] = nil
	}
	brs.idles = brs.idles[:m]
	brs.locker.Unlock()

	tmp := *scratch
	for j := range tmp {
		brs.locker.Lock()
		brs.closeBufferAndRing(tmp[j])
		brs.locker.Unlock()
		tmp[j] = nil
	}

	return

}

func (brs *BufferAndRings) Unregister() error {
	for _, br := range brs.idles {
		// free br
		_ = br.free(brs.eventLoop.ring)
		// recycle br
		brs.recycleBufferAndRing(br)
	}
	return nil
}
