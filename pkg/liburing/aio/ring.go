//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/queue"
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"os"
	"strconv"
	"strings"
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
	Submit(op *Operation) bool
	DirectAllocEnabled() bool
	RegisterFixedFdEnabled() bool
	RegisterFixedFd(fd int) (index int, err error)
	UnregisterFixedFd(index int) (err error)
	AcquireBuffer() *FixedBuffer
	ReleaseBuffer(buf *FixedBuffer)
	Close() (err error)
}

func OpenIOURing(options Options) (v IOURing, err error) {
	// version check
	if !liburing.VersionEnable(5, 19, 0) {
		err = NewRingErr(errors.New("kernel version must >= 5.19"))
		return
	}
	// probe
	probe, probeErr := liburing.GetProbe()
	if probeErr != nil {
		err = NewRingErr(probeErr)
	}
	// ring
	if options.Flags&liburing.SetupSQPoll != 0 && options.SQThreadIdle == 0 { // set default idle
		options.SQThreadIdle = 10000
	}
	if options.Flags&liburing.SetupSQAff != 0 {
		if err = sys.MaskCPU(int(options.SQThreadCPU)); err != nil { // mask cpu when sq_aff set
			err = NewRingErr(err)
			return
		}
	}
	opts := make([]liburing.Option, 0, 1)
	opts = append(opts, liburing.WithEntries(options.Entries))
	opts = append(opts, liburing.WithFlags(options.Flags))
	opts = append(opts, liburing.WithSQThreadIdle(options.SQThreadIdle))
	opts = append(opts, liburing.WithSQThreadCPU(options.SQThreadCPU))
	if options.AttachRingFd > 0 {
		opts = append(opts, liburing.WithAttachWQFd(uint32(options.AttachRingFd)))
	}
	ring, ringErr := liburing.New(opts...)
	if ringErr != nil {
		err = NewRingErr(ringErr)
		return
	}

	// register files
	var (
		generic              = liburing.GenericVersion()
		registerFiledEnabled = liburing.VersionEnable(6, 0, 0)                                                 // support io_uring_prep_cancel_fd(IORING_ASYNC_CANCEL_FD_FIXED)
		directAllocEnabled   = liburing.VersionEnable(6, 7, 0) && probe.IsSupported(liburing.OPFixedFdInstall) // support io_uring_prep_cmd_sock(SOCKET_URING_OP_SETSOCKOPT) and io_uring_prep_fixed_fd_install
		files                []int
		fileIndexes          = queue.New[int]()
		reservedHolds        []int
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
			// register files
			if _, regErr := ring.RegisterFilesSparse(options.RegisterFixedFiles); regErr != nil {
				_ = ring.Close()
				err = NewRingErr(regErr)
				return
			}
			if !generic { // wsl2 is not support well, use reserved to make listen good.
				if options.RegisterFixedFilesReserved == 0 {
					options.RegisterFixedFilesReserved = 8
				}
				if options.RegisterFixedFilesReserved*4 >= options.RegisterFixedFiles {
					_, _ = ring.UnregisterFiles()
					_ = ring.Close()
					err = NewRingErr(errors.New("reserved fixed files too big"))
					return
				}
				// reserved
				files = make([]int, options.RegisterFixedFilesReserved)
				reservedHolds = make([]int, options.RegisterFixedFilesReserved)
				for i := uint32(0); i < options.RegisterFixedFilesReserved; i++ {
					reservedHold, _ := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_RAW|syscall.SOCK_NONBLOCK|syscall.SOCK_CLOEXEC, 0)
					reservedHolds[i] = reservedHold
					files[i] = reservedHold
					idx := int(i)
					fileIndexes.Enqueue(&idx)
				}
				// keep reserved
				if _, regErr := ring.RegisterFilesUpdate(0, files); regErr != nil {
					_ = ring.Close()
					err = NewRingErr(regErr)
					return
				}
				if _, regErr := ring.RegisterFileAllocRange(options.RegisterFixedFilesReserved, options.RegisterFixedFiles-options.RegisterFixedFilesReserved); regErr != nil {
					_ = ring.Close()
					err = NewRingErr(regErr)
					return
				}
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
			files = make([]int, options.RegisterFixedFiles)
			for i := uint32(0); i < options.RegisterFixedFiles; i++ {
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
	buffers := queue.New[FixedBuffer]()
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

	// producer
	var (
		producerLockOSThread    = options.SQEProducerLockOSThread
		producerBatchSize       = options.SQEProducerBatchSize
		producerBatchTimeWindow = options.SQEProducerBatchTimeWindow
		producerBatchIdleTime   = options.SQEProducerBatchIdleTime
	)
	if ring.Flags()&liburing.SetupSingleIssuer != 0 {
		producerLockOSThread = true
	}
	producer := newSQEChanProducer(ring, producerLockOSThread, int(producerBatchSize), producerBatchTimeWindow, producerBatchIdleTime)

	// consumer
	consumerType := strings.ToUpper(strings.TrimSpace(options.CQEConsumerType))
	var consumer CQEConsumer
	switch consumerType {
	case CQEConsumerPushType:
		consumer, err = newCQEPushTypedConsumer(ring)
		break
	default:
		consumer, err = newCQEPollTypedConsumer(ring, options.CQEPullTypedConsumeIdleTime, options.CQEConsumeTimeCurve)
		break
	}
	if err != nil {
		_ = producer.Close()
		_ = ring.Close()
		err = NewRingErr(err)
		return
	}

	r := &Ring{
		ring:               ring,
		heartbeatTimeout:   options.HeartbeatTimeout,
		done:               make(chan struct{}),
		producer:           producer,
		consumer:           consumer,
		bufferRegistered:   buffers.Length() > 0,
		buffers:            buffers,
		directAllocEnabled: directAllocEnabled,
		fixedFileLocker:    new(sync.Mutex),
		files:              files,
		fileIndexes:        fileIndexes,
		reservedHolds:      reservedHolds,
	}

	go r.heartbeat()

	v = r
	return
}

type Ring struct {
	ring               *liburing.Ring
	heartbeatTimeout   time.Duration
	done               chan struct{}
	producer           SQEProducer
	consumer           CQEConsumer
	bufferRegistered   bool
	buffers            *queue.Queue[FixedBuffer]
	directAllocEnabled bool
	fixedFileLocker    sync.Locker
	files              []int
	fileIndexes        *queue.Queue[int]
	reservedHolds      []int
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
	_, err = r.ring.RegisterFilesUpdate(0, r.files)
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
	_, err = r.ring.RegisterFilesUpdate(0, r.files)
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

func (r *Ring) Submit(op *Operation) bool {
	return r.producer.Produce(op)
}

func (r *Ring) heartbeat() {
	if r.heartbeatTimeout < 1 {
		r.heartbeatTimeout = defaultHeartbeatTimeout
	}
	ticker := time.NewTicker(r.heartbeatTimeout)
	defer ticker.Stop()
	for {
		select {
		case <-r.done:
			return
		case <-ticker.C:
			op := &Operation{}
			_ = op.PrepareNop()
			if ok := r.Submit(op); !ok {
				break
			}
			break
		}
	}
}

func (r *Ring) Close() (err error) {
	// done
	close(r.done)
	// close producer
	_ = r.producer.Close()
	// close consumer
	_ = r.consumer.Close()
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
	// close reserved holds
	if len(r.reservedHolds) > 0 {
		for _, fd := range r.reservedHolds {
			_ = syscall.Close(fd)
		}
	}
	// close
	err = r.ring.Close()
	return
}
