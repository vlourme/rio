//go:build linux

package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/iouring/aio/sys"
	"os"
	"runtime"
	"strconv"
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
	DirectAllocEnabled() bool
	RegisterFixedFdEnabled() bool
	RegisterFixedFd(fd int) (index int, err error)
	UnregisterFixedFd(index int) (err error)
	AcquireBuffer() *FixedBuffer
	ReleaseBuffer(buf *FixedBuffer)
	Close() (err error)
}

func OpenIOURing(ctx context.Context, options Options) (v IOURing, err error) {
	// probe
	probe, probeErr := iouring.GetProbe()
	if probeErr != nil {
		err = NewRingErr(probeErr)
	}
	// ring
	opts := make([]iouring.Option, 0, 1)
	opts = append(opts, iouring.WithEntries(options.Entries))
	opts = append(opts, iouring.WithFlags(options.Flags))
	if options.Flags&iouring.SetupSQPoll != 0 && options.SQThreadIdle == 0 {
		options.SQThreadIdle = 10000
		opts = append(opts, iouring.WithSQThreadIdle(options.SQThreadIdle))
		opts = append(opts, iouring.WithSQThreadCPU(options.SQThreadCPU))
	}
	if options.AttachRingFd > 0 {
		opts = append(opts, iouring.WithAttachWQFd(uint32(options.AttachRingFd)))
	}
	ring, ringErr := iouring.New(opts...)
	if ringErr != nil {
		err = NewRingErr(ringErr)
		return
	}

	// register files
	var (
		registerFiledEnabled = iouring.VersionEnable(6, 0, 0)                                                // support io_uring_prep_cancel_fd(IORING_ASYNC_CANCEL_FD_FIXED)
		directAllocEnabled   = iouring.VersionEnable(6, 7, 0) && probe.IsSupported(iouring.OPFixedFdInstall) // support io_uring_prep_cmd_sock(SOCKET_URING_OP_SETSOCKOPT) and io_uring_prep_fixed_fd_install
		files                []int
		fileIndexes          *Queue[int]
	)

	if registerFiledEnabled {
		if directAllocEnabled { // use reserved and register files sparse
			if options.RegisterFixedFiles < 1024 {
				options.RegisterFixedFiles = 65535
			}
			if options.RegisterFixedFiles > 65535 {
				soft, _, limitErr := sys.GetRLimit()
				if limitErr != nil {
					_ = ring.Close()
					err = NewRingErr(os.NewSyscallError("getrlimit", err))
					return
				}
				if soft < uint64(options.RegisterFixedFiles) {
					_ = ring.Close()
					err = NewRingErr(errors.New("register fixed files too big, must smaller than " + strconv.FormatUint(soft, 10)))
					return
				}
			}
			if options.RegisterReservedFixedFiles == 0 {
				options.RegisterReservedFixedFiles = 8
			}
			if options.RegisterReservedFixedFiles*4 >= options.RegisterFixedFiles {
				_ = ring.Close()
				err = NewRingErr(errors.New("reserved fixed files too big"))
				return
			}
			// reserved
			files = make([]int, options.RegisterReservedFixedFiles)
			fileIndexes = NewQueue[int]()
			for i := uint32(0); i < options.RegisterReservedFixedFiles; i++ {
				files[i] = -1
				idx := int(i)
				fileIndexes.Enqueue(&idx)
			}
			// register files
			if _, regErr := ring.RegisterFilesSparse(options.RegisterFixedFiles); regErr != nil {
				_ = ring.Close()
				err = NewRingErr(regErr)
				return
			}
			// keep reserved
			if _, regErr := ring.RegisterFileAllocRange(options.RegisterReservedFixedFiles, options.RegisterFixedFiles-options.RegisterReservedFixedFiles); regErr != nil {
				_ = ring.Close()
				err = NewRingErr(regErr)
				return
			}
		} else { // use list and register files
			if options.RegisterFixedFiles == 0 {
				options.RegisterFixedFiles = 1024
			}
			if options.RegisterFixedFiles > 65535 {
				var limit syscall.Rlimit
				if err = syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
					_ = ring.Close()
					err = NewRingErr(os.NewSyscallError("getrlimit", err))
					return
				}
				if limit.Cur < uint64(options.RegisterFixedFiles) {
					_ = ring.Close()
					err = NewRingErr(errors.New("register fixed files too big, must smaller than " + strconv.FormatUint(limit.Cur, 10)))
					return
				}
			}
			files = make([]int, options.RegisterReservedFixedFiles)
			fileIndexes = NewQueue[int]()
			for i := uint32(0); i < options.RegisterReservedFixedFiles; i++ {
				files[i] = -1
				idx := int(i)
				fileIndexes.Enqueue(&idx)
			}
			// register files
			if _, regErr := ring.RegisterFiles(files); regErr != nil {
				_ = ring.Close()
				err = NewRingErr(regErr)
				return
			}
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

	// affinity cpu
	var (
		sqThreadCPU = options.SQThreadCPU
		prepAFFCPU  = options.PrepSQEBatchAffCPU
		waitAFFCPU  = options.PrepSQEBatchAffCPU
	)

	if cpus := runtime.NumCPU(); cpus > 3 {
		if prepAFFCPU == -1 {
			if ring.Flags()&iouring.SetupSQPoll != 0 {
				prepAFFCPU = int(sqThreadCPU) + 1
			}
		}
		if waitAFFCPU == -1 {
			if ring.Flags()&iouring.SetupSQPoll != 0 {
				waitAFFCPU = int(sqThreadCPU) + 2
			}
		}
	}

	r := &Ring{
		ring:                   ring,
		requestCh:              make(chan *Operation, ring.SQEntries()),
		cancel:                 nil,
		wg:                     sync.WaitGroup{},
		prepAFFCPU:             prepAFFCPU,
		prepSQEBatchSize:       options.PrepSQEBatchSize,
		prepSQEBatchTimeWindow: options.PrepSQEBatchTimeWindow,
		prepSQEIdleTime:        options.PrepSQEBatchIdleTime,
		waitAFFCPU:             waitAFFCPU,
		waitCQEBatchSize:       options.WaitCQEBatchSize,
		waitCQETimeCurve:       options.WaitCQEBatchTimeCurve,
		bufferRegistered:       buffers.Length() > 0,
		buffers:                buffers,
		directAllocEnabled:     directAllocEnabled,
		fixedFileLocker:        new(sync.Mutex),
		files:                  files,
		fileIndexes:            fileIndexes,
	}
	r.start(ctx)

	v = r
	return
}

type Ring struct {
	ring                   *iouring.Ring
	requestCh              chan *Operation
	cancel                 context.CancelFunc
	wg                     sync.WaitGroup
	prepAFFCPU             int
	prepSQEBatchSize       uint32
	prepSQEBatchTimeWindow time.Duration
	prepSQEIdleTime        time.Duration
	waitAFFCPU             int
	waitCQEBatchSize       uint32
	waitCQETimeCurve       Curve
	bufferRegistered       bool
	buffers                *Queue[FixedBuffer]
	directAllocEnabled     bool
	fixedFileLocker        sync.Locker
	files                  []int
	fileIndexes            *Queue[int]
}

func (r *Ring) Fd() int {
	return r.ring.Fd()
}

func (r *Ring) acquireFixedFd() int {
	nn := r.fileIndexes.Dequeue()
	if nn == nil {
		return -1
	}
	return *nn
}

func (r *Ring) releaseFixedFd(index int) {
	if index < 0 || index >= len(r.files) {
		return
	}
	r.fileIndexes.Enqueue(&index)
}

func (r *Ring) RegisterFixedFdEnabled() bool {
	return len(r.files) > 0
}

func (r *Ring) DirectAllocEnabled() bool {
	return r.directAllocEnabled
}

func (r *Ring) RegisterFixedFd(fd int) (index int, err error) {
	if r.fileIndexes == nil || r.fileIndexes.Length() == 0 {
		return -1, ErrFixedFileUnavailable
	}
	r.fixedFileLocker.Lock()
	defer r.fixedFileLocker.Unlock()
	if len(r.files) == 0 {
		index = -1
		err = ErrFixedFileUnregistered
		return
	}
	index = r.acquireFixedFd()
	if index < 0 {
		err = ErrFixedFileUnavailable
		return
	}
	r.files[index] = fd
	_, err = r.ring.RegisterFilesUpdate(uint(index), r.files[index:index+1])
	if err != nil {
		r.releaseFixedFd(index)
		index = -1
	}
	return
}

func (r *Ring) UnregisterFixedFd(index int) (err error) {
	if index < 0 || index >= len(r.files) {
		return errors.New("invalid index")
	}
	r.releaseFixedFd(index)
	r.fixedFileLocker.Lock()
	defer r.fixedFileLocker.Unlock()
	r.files[index] = -1
	_, err = r.ring.RegisterFilesUpdate(uint(index), r.files[index:index+1])
	return
}

func (r *Ring) AcquireBuffer() *FixedBuffer {
	if r.bufferRegistered {
		return r.buffers.Dequeue()
	}
	return nil
}

func (r *Ring) ReleaseBuffer(buf *FixedBuffer) {
	if buf == nil {
		return
	}
	if r.bufferRegistered {
		buf.Reset()
		r.buffers.Enqueue(buf)
	}
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
	// cancel prep and wait go
	r.cancel()
	r.cancel = nil
	r.wg.Wait()
	// unregister buffers
	if r.bufferRegistered {
		_, _ = r.ring.UnregisterBuffers()
		for {
			if buf := r.buffers.Dequeue(); buf == nil {
				break
			}
		}
	}
	// unregister files
	if r.RegisterFixedFdEnabled() {
		_, _ = r.ring.UnregisterFiles()
	}
	// close
	err = r.ring.Close()
	return
}
