//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"math"
	"os"
	"sync"
	"time"
	"unsafe"
)

var bufferAndRingRegisters = sync.Pool{}

func acquireBufferAndRingRegister(group *BufferAndRingGroup) *BufferAndRingRegister {
	v := bufferAndRingRegisters.Get()
	if v == nil {
		r := &BufferAndRingRegister{
			group: group,
		}
		return r
	}
	r := v.(*BufferAndRingRegister)
	r.group = group
	return r
}

func releaseBufferAndRingRegister(r *BufferAndRingRegister) {
	r.group = nil
	bufferAndRingRegisters.Put(r)
}

type BufferAndRingRegister struct {
	group *BufferAndRingGroup
}

func (r BufferAndRingRegister) Handle(n int, flags uint32, err error) (bool, int, uint32, unsafe.Pointer, error) {
	if err != nil {
		return true, n, flags, nil, err
	}
	var br *BufferAndRing = nil
	br, err = r.group.registerBufferAndRing()
	return true, n, flags, unsafe.Pointer(br), err
}

var bufferAndRingUnregisters = sync.Pool{}

func acquireBufferAndRingUnregister(br *BufferAndRing) *BufferAndRingUnregister {
	v := bufferAndRingUnregisters.Get()
	if v == nil {
		r := &BufferAndRingUnregister{
			br: br,
		}
		return r
	}
	r := v.(*BufferAndRingUnregister)
	r.br = br
	return r
}

func releaseBufferAndRingUnregister(r *BufferAndRingUnregister) {
	r.br = nil
	bufferAndRingUnregisters.Put(r)
}

type BufferAndRingUnregister struct {
	br *BufferAndRing
}

func (r BufferAndRingUnregister) Handle(n int, flags uint32, err error) (bool, int, uint32, unsafe.Pointer, error) {
	if err != nil {
		return true, n, flags, nil, err
	}
	// free
	err = r.br.free()
	return true, n, flags, nil, err
}

type BufferAndRing struct {
	bgid        uint16
	lastUseTime time.Time
	config      BufferAndRingConfig
	group       *BufferAndRingGroup
	value       *liburing.BufferAndRing
	buffer      []byte
}

func (br *BufferAndRing) Id() uint16 {
	return br.bgid
}

func (br *BufferAndRing) free() (err error) {
	// free br
	entries := uint32(br.config.Count)
	err = br.group.eventLoop.ring.FreeBufRing(br.value, entries, br.bgid)
	// recycle
	br.group.recycleBufferAndRing(br)
	return
}

const (
	maxBufferSize = int(^uint(0) >> 1)
)

func newBufferAndRingGroup(config BufferAndRingConfig) (brs *BufferAndRingGroup, err error) {
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

	if config.IdleTimeout < 5*time.Second {
		config.IdleTimeout = 15 * time.Second
	}
	config.mask = uint16(liburing.BufferRingMask(uint32(config.Count)))

	bgids := make([]uint16, math.MaxUint16)
	for i := 0; i < len(bgids); i++ {
		bgids[i] = uint16(i)
	}

	brs = &BufferAndRingGroup{
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

type BufferAndRingGroup struct {
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

func (group *BufferAndRingGroup) getBuffer() []byte {
	v := group.bufferPool.Get()
	if v == nil {
		return make([]byte, group.bufferLength)
	}
	return v.([]byte)
}

func (group *BufferAndRingGroup) putBuffer(buf []byte) {
	if len(buf) == 0 {
		return
	}
	group.bufferPool.Put(buf)
}

func (group *BufferAndRingGroup) Acquire() (br *BufferAndRing, err error) {
	group.locker.Lock()
	// no idles
	if len(group.idles) == 0 {
		group.locker.Unlock()
		// register one
		register := acquireBufferAndRingRegister(group)
		var ptr unsafe.Pointer
		op := AcquireOperation()
		op.PrepareRegisterBufferAndRing(register)
		_, _, ptr, err = group.eventLoop.Submit(op).Await()
		if err == nil {
			br = (*BufferAndRing)(ptr)
		}
		ReleaseOperation(op)
		releaseBufferAndRingRegister(register)
		return
	}

	// use an idle one
	br = group.idles[0]
	group.idles = group.idles[1:]

	group.locker.Unlock()
	return
}

func (group *BufferAndRingGroup) Release(br *BufferAndRing) {
	if br == nil {
		return
	}

	group.locker.Lock()
	br.lastUseTime = time.Now()
	group.idles = append(group.idles, br)
	group.locker.Unlock()
	return
}

func (group *BufferAndRingGroup) registerBufferAndRing() (value *BufferAndRing, err error) {
	group.locker.Lock()
	if len(group.bgids) == 0 {
		err = errors.New("register buffer and ring failed cause no bgid available")
		group.locker.Unlock()
		return
	}

	bgid := group.bgids[0]

	entries := uint32(group.config.Count)
	br, setupErr := group.eventLoop.ring.SetupBufRing(entries, bgid, 0)
	if setupErr != nil {
		err = setupErr
		group.locker.Unlock()
		return
	}

	group.bgids = group.bgids[1:]
	group.locker.Unlock()

	mask := group.config.mask
	buffer := group.getBuffer()
	bufferUnitLength := uint32(group.config.Size)
	for i := uint32(0); i < entries; i++ {
		beg := bufferUnitLength * i
		end := beg + bufferUnitLength
		slice := buffer[beg:end]
		addr := &slice[0]
		br.BufRingAdd(unsafe.Pointer(addr), bufferUnitLength, uint16(i), mask, uint16(i))
	}
	br.BufRingAdvance(uint16(entries))

	value = &BufferAndRing{
		bgid:        bgid,
		lastUseTime: time.Time{},
		config:      group.config,
		group:       group,
		value:       br,
		buffer:      buffer,
	}
	return
}

func (group *BufferAndRingGroup) unregisterBufferAndRing(br *BufferAndRing) {
	unregister := acquireBufferAndRingUnregister(br)
	op := AcquireOperation()
	op.PrepareUnregisterBufferAndRing(unregister)
	_, _, _ = group.eventLoop.SubmitAndWait(op)
	ReleaseOperation(op)
	releaseBufferAndRingUnregister(unregister)
	return
}

func (group *BufferAndRingGroup) recycleBufferAndRing(br *BufferAndRing) {
	group.locker.Lock()
	// release bgid
	group.bgids = append(group.bgids, br.bgid)
	// release buffer
	buffer := br.buffer
	br.buffer = nil
	group.putBuffer(buffer)
	group.locker.Unlock()
	return
}

func (group *BufferAndRingGroup) Start(eventLoop *EventLoop) {
	group.eventLoop = eventLoop
	group.wg.Add(1)
	go func(brs *BufferAndRingGroup) {
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
	}(group)
}

func (group *BufferAndRingGroup) Stop() {
	close(group.done)
	group.wg.Wait()
}

func (group *BufferAndRingGroup) clean(scratch *[]*BufferAndRing) {
	group.locker.Lock()

	n := len(group.idles)
	if n == 0 {
		group.locker.Unlock()
		return
	}

	maxIdleDuration := group.config.IdleTimeout
	criticalTime := time.Now().Add(-maxIdleDuration)

	l, r, mid := 0, n-1, 0
	for l <= r {
		mid = (l + r) / 2
		if criticalTime.After(group.idles[mid].lastUseTime) {
			l = mid + 1
		} else {
			r = mid - 1
		}
	}
	i := r
	if i == -1 {
		group.locker.Unlock()
		return
	}

	*scratch = append((*scratch)[:0], group.idles[:i+1]...)
	m := copy(group.idles, group.idles[i+1:])
	for i = m; i < n; i++ {
		group.idles[i] = nil
	}
	group.idles = group.idles[:m]
	group.locker.Unlock()

	tmp := *scratch
	for j := range tmp {
		group.unregisterBufferAndRing(tmp[j])
		tmp[j] = nil
	}

	return

}

func (group *BufferAndRingGroup) Unregister() error {
	for _, br := range group.idles {
		_ = br.free()
	}
	return nil
}
