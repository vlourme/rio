//go:build linux

package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/sys"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type IOURing interface {
	Start(ctx context.Context)
	Submit(op *Operation) (ok bool)
	RegisterFixedFdEnabled() bool
	GetRegisterFixedFd(index int) int
	RegisterFixedFd(fd int) (index int, err error)
	UnregisterFixedFd(index int) (err error)
	AcquireBuffer() *FixedBuffer
	ReleaseBuffer(buf *FixedBuffer)
	OpSupported(op uint8) bool
	Close() (err error)
}

func NewIOURing(options Options) (r IOURing, err error) {
	// ring
	if options.Flags&iouring.SetupSQPoll != 0 && options.SQThreadIdle == 0 {
		options.SQThreadIdle = 10000
	}
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

	// probe
	probe, probeErr := ring.Probe()
	if probeErr != nil {
		_ = ring.Close()
		err = probeErr
		return
	}

	// register files
	var fixedFiles []int
	fixedFileIndexes := NewQueue[int]()
	options.RegisterFixedFiles = 10
	if files := options.RegisterFixedFiles; files > 0 {
		soft, _, limitErr := sys.GetRLimit()
		if limitErr != nil {
			err = limitErr
			return
		}
		if uint64(files) > soft {
			files = uint32(soft)
		}
		fixedFiles = make([]int, files)
		for i := range fixedFiles {
			fixedFiles[i] = -1
			idx := i
			fixedFileIndexes.Enqueue(&idx)
		}
		_, regErr := ring.RegisterFiles(fixedFiles)
		if regErr != nil {
			_ = ring.Close()
			err = regErr
			return
		}
	}

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
		probe:                  probe,
		requestCh:              make(chan *Operation, ring.SQEntries()),
		buffers:                buffers,
		cancel:                 nil,
		wg:                     sync.WaitGroup{},
		fixedBufferRegistered:  buffers.Length() > 0,
		prepAFFCPU:             options.PrepSQEBatchAffCPU,
		prepSQEBatchSize:       options.PrepSQEBatchSize,
		prepSQEBatchTimeWindow: options.PrepSQEBatchTimeWindow,
		prepSQEIdleTime:        options.PrepSQEBatchIdleTime,
		waitAFFCPU:             options.WaitCQEBatchAffCPU,
		waitCQEBatchSize:       options.WaitCQEBatchSize,
		waitCQETimeCurve:       options.WaitCQEBatchTimeCurve,
		fixedFileLocker:        new(sync.RWMutex),
		fixedFileIndexes:       fixedFileIndexes,
		fixedFiles:             fixedFiles,
	}
	return
}

type Ring struct {
	serving                atomic.Bool
	ring                   *iouring.Ring
	probe                  *iouring.Probe
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
	fixedFileLocker        *sync.RWMutex
	fixedFileIndexes       *Queue[int]
	fixedFiles             []int
}

func (r *Ring) Fd() int {
	return r.ring.Fd()
}

func (r *Ring) acquireFixedFileIndex() int {
	nn := r.fixedFileIndexes.Dequeue()
	if nn == nil {
		return -1
	}
	return *nn
}

func (r *Ring) releaseFixedFileIndex(index int) {
	if index < 0 || index >= len(r.fixedFiles) {
		return
	}
	r.fixedFileIndexes.Enqueue(&index)
}

func (r *Ring) RegisterFixedFdEnabled() bool {
	return len(r.fixedFiles) > 0
}

func (r *Ring) GetRegisterFixedFd(index int) int {
	r.fixedFileLocker.RLock()
	defer r.fixedFileLocker.RUnlock()
	if index < 0 || index >= len(r.fixedFiles) {
		return -1
	}
	return r.fixedFiles[index]
}

func (r *Ring) RegisterFixedFd(fd int) (index int, err error) {
	r.fixedFileLocker.Lock()
	defer r.fixedFileLocker.Unlock()
	if len(r.fixedFiles) == 0 {
		err = errors.New("files has not been registered yet")
		return
	}
	index = r.acquireFixedFileIndex()
	if index < 0 {
		err = errors.New("no files available")
		return
	}
	r.fixedFiles[index] = fd
	_, err = r.ring.RegisterFilesUpdate(uint(index), r.fixedFiles[index:index+1])
	if err != nil {
		index = -1
	}
	return
}

func (r *Ring) UnregisterFixedFd(index int) (err error) {
	r.fixedFileLocker.Lock()
	defer r.fixedFileLocker.Unlock()
	if index < 0 || index >= len(r.fixedFiles) {
		return errors.New("invalid index")
	}
	// todo check need to update files
	r.releaseFixedFileIndex(index)
	return
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

func (r *Ring) OpSupported(op uint8) bool {
	return r.probe.IsSupported(op)
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
	if len(r.fixedFiles) > 0 {
		_, _ = r.ring.UnregisterFiles()
	}
	err = r.ring.Close()
	return
}
