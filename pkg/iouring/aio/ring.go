//go:build linux

package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"os"
	"sync"
	"syscall"
	"time"
)

var (
	ErrFixedFileUnavailable  = errors.New("fixed files unavailable")
	ErrFixedFileUnregistered = errors.New("fixed files unregistered")
)

type IOURing interface {
	Fd() int
	Submit(op *Operation)
	RegisterFixedFdEnabled() bool
	GetRegisterFixedFd(index int) int
	PopFixedFd() (index int, err error)
	RegisterFixedFd(fd int) (index int, err error)
	UnregisterFixedFd(index int) (err error)
	AcquireBuffer() *FixedBuffer
	ReleaseBuffer(buf *FixedBuffer)
	OpSupported(op uint8) bool
	Close() (err error)
}

func OpenIOURing(ctx context.Context, options Options) (v IOURing, err error) {
	// ring
	if options.Flags&iouring.SetupSQPoll != 0 && options.SQThreadIdle == 0 {
		options.SQThreadIdle = 10000
	}
	opts := make([]iouring.Option, 0, 1)
	opts = append(opts, iouring.WithEntries(options.Entries))
	opts = append(opts, iouring.WithFlags(options.Flags))
	opts = append(opts, iouring.WithSQThreadIdle(options.SQThreadIdle))
	opts = append(opts, iouring.WithSQThreadCPU(options.SQThreadCPU))
	if options.AttachRingFd > 0 {
		opts = append(opts, iouring.WithAttachWQFd(uint32(options.AttachRingFd)))
	}
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
	if options.RegisterFixedFiles == 0 {
		options.RegisterFixedFiles = 4096
	}
	var fixedFiles []int
	fixedFileIndexes := NewQueue[int]()
	if files := options.RegisterFixedFiles; files > 0 {
		var limit syscall.Rlimit
		if err = syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
			_ = ring.Close()
			err = os.NewSyscallError("getrlimit", err)
			return
		}
		if uint64(files) > limit.Cur {
			files = uint32(limit.Cur)
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

	r := &Ring{
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
	r.start(ctx)

	v = r
	return
}

type Ring struct {
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

func (r *Ring) PopFixedFd() (index int, err error) {
	if r.fixedFileIndexes.Length() == 0 {
		return -1, ErrFixedFileUnavailable
	}
	r.fixedFileLocker.Lock()
	defer r.fixedFileLocker.Unlock()
	if len(r.fixedFiles) == 0 {
		index = -1
		err = ErrFixedFileUnregistered
		return
	}
	index = r.acquireFixedFileIndex()
	if index < 0 {
		err = ErrFixedFileUnavailable
		return
	}
	return
}

func (r *Ring) RegisterFixedFd(fd int) (index int, err error) {
	if r.fixedFileIndexes.Length() == 0 {
		return -1, ErrFixedFileUnavailable
	}
	r.fixedFileLocker.Lock()
	defer r.fixedFileLocker.Unlock()
	if len(r.fixedFiles) == 0 {
		index = -1
		err = ErrFixedFileUnregistered
		return
	}
	index = r.acquireFixedFileIndex()
	if index < 0 {
		err = ErrFixedFileUnavailable
		return
	}
	r.fixedFiles[index] = fd
	_, err = r.ring.RegisterFilesUpdate(uint(index), r.fixedFiles[index:index+1])
	if err != nil {
		r.releaseFixedFileIndex(index)
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

func (r *Ring) Submit(op *Operation) {
	r.requestCh <- op
	return
}

func (r *Ring) start(ctx context.Context) {
	cc, cancel := context.WithCancel(ctx)
	r.cancel = cancel

	r.wg.Add(2)
	go r.preparingSQE(cc)
	go r.waitingCQE(cc)
	return
}

func (r *Ring) Close() (err error) {
	r.cancel()
	r.cancel = nil
	r.wg.Wait()
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
