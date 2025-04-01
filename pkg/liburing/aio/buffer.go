package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"io"
	"math"
	"os"
	"slices"
	"sync"
	"time"
	"unsafe"
)

type BufferAndRingConfig struct {
	Size        uint16
	Count       uint16
	Reference   uint16
	IdleTimeout time.Duration
}

type BufferAndRing struct {
	bgid         uint16
	value        *liburing.BufferAndRing
	reference    uint16
	maxReference uint16
	size         uint16
	lastIdleTime time.Time
	buffer       []byte
}

func (br *BufferAndRing) Id() uint16 {
	return br.bgid
}

func (br *BufferAndRing) WriteTo(length int, cqeFlags uint32, writer io.Writer) (n int, err error) {
	if cqeFlags == 0 {
		return
	}

	bid := uint16(cqeFlags >> liburing.IORING_CQE_BUFFER_SHIFT)
	if length == 0 {
		br.value.BufRingAdvance(1)
		return
	}

	idx := int(bid * br.size)
	b := br.buffer[idx : idx+length]
	nn := 0
	for {
		nn, err = writer.Write(b[nn:])
		if err != nil {
			break
		}
		n += nn
		if n == length {
			break
		}
	}

	used := uint16(math.Ceil(float64(length) / float64(br.size)))

	br.value.BufRingAdvance(used)
	return
}

func (br *BufferAndRing) incrReference() bool {
	if br.reference == br.maxReference {
		return false
	}
	br.reference++
	return true
}

func (br *BufferAndRing) decrReference() {
	if br.reference == 0 {
		return
	}
	br.reference--
	if br.reference == 0 {
		br.lastIdleTime = time.Now()
	}
}

func (br *BufferAndRing) idle() bool {
	return br.reference == 0
}

const (
	maxBufferSize = int(^uint(0) >> 1)
)

func newBufferAndRings(ring *liburing.Ring, config BufferAndRingConfig) (brs *BufferAndRings, err error) {
	size := config.Size
	if size == 0 {
		size = uint16(os.Getpagesize())
	}
	size = uint16(liburing.RoundupPow2(uint32(size)))
	config.Size = size

	count := config.Count
	if count == 0 {
		count = 16
	}
	count = uint16(liburing.RoundupPow2(uint32(count)))
	config.Count = count

	ref := config.Reference
	if ref == 0 {
		ref = 512
	}
	ref = uint16(liburing.RoundupPow2(uint32(ref)))
	config.Reference = ref

	if count*ref > 32768 {
		err = errors.New("count and reference are too large for BufferAndRings, max of count * reference is 32768")
		return
	}

	bLen := size * count * ref
	if int(bLen) > maxBufferSize {
		err = errors.New("size, count and ref is too large for BufferAndRings")
		return
	}

	if config.IdleTimeout < 1 {
		config.IdleTimeout = 10 * time.Second
	}

	brs = &BufferAndRings{
		config:     config,
		locker:     sync.Mutex{},
		ring:       ring,
		wg:         &sync.WaitGroup{},
		done:       make(chan struct{}),
		bgids:      make([]uint16, 0, 8),
		values:     make([]*BufferAndRing, 0, 8),
		bufferPool: sync.Pool{},
	}
	brs.bufferPool.New = func() interface{} {
		return make([]byte, brs.config.Size*brs.config.Count*brs.config.Reference)
	}

	brs.start()
	return
}

type BufferAndRings struct {
	config     BufferAndRingConfig
	locker     sync.Mutex
	ring       *liburing.Ring
	wg         *sync.WaitGroup
	done       chan struct{}
	bgids      []uint16
	values     []*BufferAndRing
	bufferPool sync.Pool
}

func (brs *BufferAndRings) Acquire() (br *BufferAndRing, err error) {
	brs.locker.Lock()

	// find one
	for _, bgid := range brs.bgids {
		value := brs.values[bgid]
		if value.incrReference() {
			br = value
			brs.locker.Unlock()
			return
		}
	}
	// find min bgid to setup
	bgid := uint16(0)
	bgidsLen := uint16(len(brs.bgids))
	switch bgidsLen {
	case 0, 1:
		bgid = bgidsLen
		break
	default:
		found := false
		for i := uint16(1); i < bgidsLen; i++ {
			bgidPrev := brs.bgids[i-1]
			bgidCurrent := brs.bgids[i]

			if delta := bgidCurrent - bgidPrev; delta == 1 {
				continue
			}
			bgid = bgidPrev + 1
			found = true
			break
		}
		if !found { // use tail
			if bgidsLen > math.MaxUint16 {
				err = errors.New("no remains bgid to setup buffer and ring")
				brs.locker.Unlock()
				return
			}
			bgid = bgidsLen
		}
		break
	}

	entries := brs.config.Count * brs.config.Reference
	br0, setupErr := brs.ring.SetupBufRing(entries, bgid, 0)
	if setupErr != nil {
		err = setupErr
		brs.locker.Unlock()
		return
	}
	mask := liburing.BufferRingMask(entries)
	buffer := brs.bufferPool.Get().([]byte)
	for i := uint16(0); i < entries; i++ {
		beg := brs.config.Size * i
		end := beg + brs.config.Size
		addr := &buffer[beg:end][0]
		br0.BufRingAdd(uintptr(unsafe.Pointer(addr)), brs.config.Size, i, mask, i)
	}
	br0.BufRingAdvance(entries)

	br = &BufferAndRing{
		bgid:         bgid,
		value:        br0,
		reference:    1,
		maxReference: brs.config.Reference,
		size:         brs.config.Size,
		lastIdleTime: time.Time{},
		buffer:       buffer,
	}

	if bgid < bgidsLen {
		brs.values[bgid] = br
	} else {
		brs.values = append(brs.values, br)
	}

	brs.bgids = append(brs.bgids, bgid)
	slices.Sort(brs.bgids)

	brs.locker.Unlock()
	return
}

func (brs *BufferAndRings) Release(br *BufferAndRing) {
	if br == nil {
		return
	}

	brs.locker.Lock()

	br.decrReference()

	brs.locker.Unlock()
	return
}

func (brs *BufferAndRings) start() {
	brs.wg.Add(1)
	go func(brs *BufferAndRings) {
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

		brs.locker.Lock()

		for _, br := range brs.values {
			// release buffer
			buffer := br.buffer
			brs.bufferPool.Put(buffer)
			// munmap
			entries := brs.config.Count * brs.config.Reference
			ringSizeAddr := uintptr(entries) * unsafe.Sizeof(liburing.BufferAndRing{})
			_ = sys.MunmapPtr(uintptr(unsafe.Pointer(br.value)), ringSizeAddr)
			// unregister
			_, _ = brs.ring.UnregisterBufferRing(br.bgid)
		}
		brs.values = nil
		brs.bgids = nil

		brs.locker.Unlock()

		brs.wg.Done()
	}(brs)

}

func (brs *BufferAndRings) clean(scratch *[]*BufferAndRing) {
	brs.locker.Lock()

	n := len(brs.values)
	if n == 0 {
		brs.locker.Unlock()
		return
	}

	maxIdleDuration := brs.config.IdleTimeout
	criticalTime := time.Now().Add(-maxIdleDuration)

	for i := 0; i < n; i++ {
		br := brs.values[i]
		if br.idle() {
			if criticalTime.After(br.lastIdleTime) {
				brs.values[i] = nil
				if bgidIdx, has := slices.BinarySearch(brs.bgids, br.bgid); has {
					brs.bgids = append(brs.bgids[:bgidIdx], brs.bgids[bgidIdx+1:]...)
				}
				brs.bgids = append(brs.bgids[:1], brs.bgids[2:]...)
				*scratch = append(*scratch, br)
			}
		}
	}

	tmp := *scratch
	for i := range tmp {
		br := tmp[i]
		// release buffer
		buffer := br.buffer
		brs.bufferPool.Put(buffer)
		// munmap
		entries := brs.config.Count * brs.config.Reference
		ringSizeAddr := uintptr(entries) * unsafe.Sizeof(liburing.BufferAndRing{})
		_ = sys.MunmapPtr(uintptr(unsafe.Pointer(br.value)), ringSizeAddr)
		// unregister
		_, _ = brs.ring.UnregisterBufferRing(br.bgid)
	}

	brs.locker.Unlock()
	return

}

func (brs *BufferAndRings) Close() error {
	close(brs.done)
	brs.wg.Wait()
	return nil
}
