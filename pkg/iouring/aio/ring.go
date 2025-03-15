//go:build linux

package aio

import (
	"context"
	"fmt"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/semaphores"
	"sync"
	"sync/atomic"
	"syscall"
)

type IOURing interface {
	Start(ctx context.Context)
	Submit(op *Operation)
	FileFd(index int) int
	AcquireBuffer() *FixedBuffer
	ReleaseBuffer(buf *FixedBuffer)
	Close() (err error)
}

func NewIOURing(options Options) (r IOURing, err error) {
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
		serving:               atomic.Bool{},
		ring:                  ring,
		requests:              NewQueue[Operation](),
		buffers:               buffers,
		submitSemaphores:      submitSemaphores,
		cancel:                nil,
		wg:                    sync.WaitGroup{},
		fixedBufferRegistered: buffers.Length() > 0,
		prepAFFCPU:            options.PrepSQEAffCPU,
		prepSQEBatchSize:      options.PrepSQEBatchSize,
		waitAFFCPU:            options.WaitCQEAffCPU,
		waitCQEBatchSize:      options.WaitCQEBatchSize,
		waitCQETimeCurve:      options.WaitCQETimeCurve,
		registeredFiles:       nil,
	}
	return
}

type Ring struct {
	serving               atomic.Bool
	ring                  *iouring.Ring
	requests              *Queue[Operation]
	buffers               *Queue[FixedBuffer]
	submitSemaphores      *semaphores.Semaphores
	cancel                context.CancelFunc
	wg                    sync.WaitGroup
	fixedBufferRegistered bool
	prepAFFCPU            int
	prepSQEBatchSize      uint32
	waitAFFCPU            int
	waitCQEBatchSize      uint32
	waitCQETimeCurve      Curve
	registeredFiles       []int
}

func (r *Ring) Fd() int {
	return r.ring.Fd()
}

func (r *Ring) FileFd(index int) int {
	if index < 0 || index >= len(r.registeredFiles) {
		return -1
	}

	fmt.Println(r.registeredFiles)
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

func (r *Ring) Submit(op *Operation) {
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
	if len(r.registeredFiles) > 0 {
		_, _ = r.ring.UnregisterFiles()
	}
	err = r.ring.Close()
	return
}
