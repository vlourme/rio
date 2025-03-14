//go:build linux

package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/semaphores"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type IOURing interface {
	Start(ctx context.Context)
	Id() int
	Submit(op *Operation)
	AcquireBuffer() *FixedBuffer
	ReleaseBuffer(buf *FixedBuffer)
	Close() (err error)
}

type RingOptions struct {
	Entries                  uint32
	Flags                    uint32
	SQThreadCPU              uint32
	SQThreadIdle             uint32
	RegisterFixedBufferSize  uint32
	RegisterFixedBufferCount uint32
	PrepSQEBatchSize         uint32
	PrepSQEIdleTime          time.Duration
	PrepSQEAffCPU            int
	WaitCQEBatchSize         uint32
	WaitCQETimeCurve         Curve
	WaitCQEAffCPU            int
}

func NewIOURing(options Options) (uring IOURing, err error) {
	num := options.RingNum
	if num == 0 {
		num = uint32(runtime.NumCPU() / 2)
	}
	if num == 1 {
		uring, err = newRing(0, 0, options)
		return
	}

	members := make([]*Ring, num)
	// master
	master, masterErr := newRing(0, 0, options)
	if masterErr != nil {
		err = masterErr
		return
	}
	members[0] = master
	// slaver
	for i := uint32(0); i < num; i++ {
		slaver, slaverErr := newRing(int(i), master.Fd(), options)
		if slaverErr != nil {
			err = slaverErr
			break
		}
		members[i] = slaver
	}
	if err != nil {
		for _, m := range members {
			if m == nil {
				break
			}
			_ = m.Close()
		}
		return
	}
	uring = &Rings{
		members: members,
		length:  uint64(len(members)),
		idx:     atomic.Uint64{},
	}
	return
}

type Rings struct {
	members []*Ring
	length  uint64
	idx     atomic.Uint64
}

func (rs *Rings) next() (r *Ring) {
	idx := (rs.idx.Add(1) - 1) % rs.length
	r = rs.members[idx]
	return
}

func (rs *Rings) Id() int {
	r := rs.next()
	return r.id
}

func (rs *Rings) Submit(op *Operation) {
	if op.ringId == -1 {
		r := rs.next()
		r.Submit(op)
		return
	}
	for i := range rs.members {
		m := rs.members[i]
		if m.id == op.ringId {
			m.Submit(op)
			break
		}
	}
	return
}

func (rs *Rings) AcquireBuffer() *FixedBuffer {
	r := rs.next()
	buf := r.AcquireBuffer()
	return buf
}

func (rs *Rings) ReleaseBuffer(buf *FixedBuffer) {
	for i := range rs.members {
		m := rs.members[i]
		if m.id == buf.ringId {
			m.ReleaseBuffer(buf)
			break
		}
	}
	return
}

func (rs *Rings) Start(ctx context.Context) {
	for _, ring := range rs.members {
		ring.Start(ctx)
	}
}

func (rs *Rings) Close() (err error) {
	for i := uint64(1); i < rs.length; i++ {
		member := rs.members[i]
		if closeErr := member.Close(); closeErr != nil {
			if err == nil {
				err = closeErr
			} else {
				err = errors.Join(err, closeErr)
			}
		}
	}
	closeErr := rs.members[0].Close()
	if closeErr != nil {
		if err == nil {
			err = closeErr
		} else {
			err = errors.Join(err, closeErr)
		}
	}
	return
}

func newRing(id int, attach int, options Options) (r *Ring, err error) {
	// op semaphores
	prepareIdleTime := options.PrepSQEIdleTime
	if prepareIdleTime < 1 {
		prepareIdleTime = defaultPrepareSQEIdleTime
	}
	submitSemaphores, submitSemaphoresErr := semaphores.New(prepareIdleTime)
	if submitSemaphoresErr != nil {
		err = submitSemaphoresErr
		return
	}
	// ring
	opts := make([]iouring.Option, 0, 1)
	opts = append(opts, iouring.WithEntries(options.Entries))
	opts = append(opts, iouring.WithFlags(options.Flags))
	if attach == 0 {
		opts = append(opts, iouring.WithSQThreadIdle(options.SQThreadIdle))
		opts = append(opts, iouring.WithSQThreadCPU(options.SQThreadCPU))
	} else {
		opts = append(opts, iouring.WithAttachWQFd(uint32(attach)))
	}
	ring, ringErr := iouring.New(opts...)

	if ringErr != nil {
		err = ringErr
		return
	}

	// register buffers
	buffers := NewQueue[FixedBuffer]()
	if size, count := options.RegisterFixedBufferSize, options.RegisterFixedBufferCount; count > 0 && size > 0 {
		iovecs := make([]syscall.Iovec, count)
		for i := uint32(0); i < count; i++ {
			buf := make([]byte, size)
			buffers.Enqueue(&FixedBuffer{
				value:  buf,
				index:  int(i),
				ringId: id,
			})
			iovecs[i] = syscall.Iovec{
				Base: &buf[0],
				Len:  uint64(size),
			}
		}
		_, regErr := ring.RegisterBuffers(iovecs)
		if regErr != nil {
			for {
				if buf := buffers.Dequeue(); buf == nil {
					break
				}
			}
		}
	}
	// todo register files

	r = &Ring{
		id:                    id,
		serving:               atomic.Bool{},
		ring:                  ring,
		requests:              NewQueue[Operation](),
		buffers:               buffers,
		submitSemaphores:      submitSemaphores,
		cancel:                nil,
		wg:                    sync.WaitGroup{},
		fixedBufferRegistered: buffers.Length() > 0,
		prepSQEBatchSize:      options.PrepSQEBatchSize,
		waitCQEBatchSize:      options.WaitCQEBatchSize,
		waitCQETimeCurve:      options.WaitCQETimeCurve,
	}
	return
}

type Ring struct {
	id                    int
	serving               atomic.Bool
	ring                  *iouring.Ring
	requests              *Queue[Operation]
	buffers               *Queue[FixedBuffer]
	submitSemaphores      *semaphores.Semaphores
	cancel                context.CancelFunc
	wg                    sync.WaitGroup
	fixedBufferRegistered bool
	prepSQEBatchSize      uint32
	waitCQEBatchSize      uint32
	waitCQETimeCurve      Curve
}

func (r *Ring) Fd() int {
	return r.ring.Fd()
}

func (r *Ring) Id() int {
	return r.id
}

func (r *Ring) AcquireBuffer() *FixedBuffer {
	if r.fixedBufferRegistered {
		return r.buffers.Dequeue()
	}
	return nil
}

func (r *Ring) ReleaseBuffer(buf *FixedBuffer) {
	if buf == nil {
		return
	}
	if r.fixedBufferRegistered {
		buf.Reset()
		r.buffers.Enqueue(buf)
	}
}

func (r *Ring) Submit(op *Operation) {
	op.ringId = r.id
	r.requests.Enqueue(op)
	r.submitSemaphores.Signal()
}

func (r *Ring) Start(ctx context.Context) {
	if r.serving.CompareAndSwap(false, true) {
		cc, cancel := context.WithCancel(ctx)
		r.cancel = cancel

		r.wg.Add(2)
		go r.preparingSQE(cc)
		go r.waitingCQE(cc)
	}
	return
}

func (r *Ring) Close() (err error) {
	if r.serving.CompareAndSwap(true, false) {
		r.cancel()
		r.cancel = nil
		r.wg.Wait()
	}
	if r.fixedBufferRegistered {
		_, _ = r.ring.UnregisterBuffers()
		for {
			if buf := r.buffers.Dequeue(); buf == nil {
				break
			}
		}
	}
	err = r.ring.Close()
	return
}
