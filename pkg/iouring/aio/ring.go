//go:build linux

package aio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/iouring"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type IOURing interface {
	Start(ctx context.Context)
	Submit(op *Operation) (ok bool)
	FileFd(index int) int
	AcquireBuffer() *FixedBuffer
	ReleaseBuffer(buf *FixedBuffer)
	Close() (err error)
}

func NewIOURing(options Options) (r IOURing, err error) {
	// ring
	opts := make([]iouring.Option, 0, 1)
	opts = append(opts, iouring.WithEntries(options.Entries))
	opts = append(opts, iouring.WithFlags(options.Flags))
	opts = append(opts, iouring.WithSQThreadIdle(options.SQThreadIdle))
	opts = append(opts, iouring.WithSQThreadCPU(options.SQThreadCPU))
	ring, ringErr := iouring.New(opts...)

	if ringErr != nil {
		err = ringErr
		return
	}
	// todo : register files 65535 by options
	//files := make([]int, 10)
	//for i := range files {
	//	files[i] = -1
	//}
	//_, regFilesErr := ring.RegisterFiles(files)
	//if regFilesErr != nil {
	//	_ = ring.Close()
	//	err = regFilesErr
	//	return
	//}
	// register buffers
	buffers := NewQueue[FixedBuffer]()
	if size, count := options.RegisterFixedBufferSize, options.RegisterFixedBufferCount; count > 0 && size > 0 {
		iovecs := make([]syscall.Iovec, count)
		for i := uint32(0); i < count; i++ {
			buf := make([]byte, size)
			buffers.Enqueue(&FixedBuffer{
				value: buf,
				index: int(i),
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

	r = &Ring{
		serving:                atomic.Bool{},
		ring:                   ring,
		requestCh:              make(chan *Operation, ring.SQEntries()),
		buffers:                buffers,
		cancel:                 nil,
		wg:                     sync.WaitGroup{},
		fixedBufferRegistered:  buffers.Length() > 0,
		prepAFFCPU:             options.PrepSQEBatchAffCPU,
		prepSQEBatchSize:       options.PrepSQEBatchSize,
		prepSQEBatchTimeWindow: 0,
		prepSQEIdleTime:        options.PrepSQEBatchIdleTime,
		waitAFFCPU:             options.WaitCQEBatchAffCPU,
		waitCQEBatchSize:       options.WaitCQEBatchSize,
		waitCQETimeCurve:       options.WaitCQEBatchTimeCurve,
		registeredFiles:        nil,
	}
	return
}

type Ring struct {
	serving                atomic.Bool
	ring                   *iouring.Ring
	requestCh              chan *Operation
	buffers                *Queue[FixedBuffer]
	cancel                 context.CancelFunc
	wg                     sync.WaitGroup
	fixedBufferRegistered  bool
	prepAFFCPU             int
	prepSQEBatchSize       uint32
	prepSQEBatchTimeWindow time.Duration
	prepSQEIdleTime        time.Duration
	waitAFFCPU             int
	waitCQEBatchSize       uint32
	waitCQETimeCurve       Curve
	registeredFiles        []int
}

func (r *Ring) Fd() int {
	return r.ring.Fd()
}

func (r *Ring) FileFd(index int) int {
	if index < 0 || index >= len(r.registeredFiles) {
		return -1
	}
	return r.registeredFiles[index]
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

func (r *Ring) Submit(op *Operation) (ok bool) {
	if ok = r.serving.Load(); ok {
		r.requestCh <- op
	}
	return
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
	if len(r.registeredFiles) > 0 {
		_, _ = r.ring.UnregisterFiles()
	}
	err = r.ring.Close()
	return
}
