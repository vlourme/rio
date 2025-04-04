//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"os"
	"strconv"
	"sync"
	"syscall"
	"time"
)

func Open(options ...Option) (v AsyncIO, err error) {
	// version check
	if !liburing.VersionEnable(5, 19, 0) { // support io_uring_setup_buf_ring
		err = NewRingErr(errors.New("kernel version must >= 5.19"))
		return
	}
	// probe
	probe, probeErr := liburing.GetProbe()
	if probeErr != nil {
		err = NewRingErr(probeErr)
	}

	// options
	opt := Options{}
	for _, option := range options {
		option(&opt)
	}
	// ring
	if opt.Flags&liburing.IORING_SETUP_SQPOLL != 0 && opt.SQThreadIdle == 0 { // set default idle
		opt.SQThreadIdle = 10000
	}
	if opt.Flags&liburing.IORING_SETUP_SQ_AFF != 0 {
		if err = sys.MaskCPU(int(opt.SQThreadCPU)); err != nil { // mask cpu when sq_aff set
			err = NewRingErr(err)
			return
		}
	}
	opts := make([]liburing.Option, 0, 1)
	opts = append(opts, liburing.WithEntries(opt.Entries))
	opts = append(opts, liburing.WithFlags(opt.Flags))
	opts = append(opts, liburing.WithSQThreadIdle(opt.SQThreadIdle))
	opts = append(opts, liburing.WithSQThreadCPU(opt.SQThreadCPU))
	if opt.AttachRingFd > 0 {
		opts = append(opts, liburing.WithAttachWQFd(uint32(opt.AttachRingFd)))
	}
	ring, ringErr := liburing.New(opts...)
	if ringErr != nil {
		err = NewRingErr(ringErr)
		return
	}

	// buffer and rings
	brs, brsErr := newBufferAndRings(ring, opt.BufferAndRingConfig)
	if brsErr != nil {
		_ = ring.Close()
		err = NewRingErr(brsErr)
		return
	}

	// register files
	var (
		directAllocEnabled = liburing.VersionEnable(6, 7, 0) && // support io_uring_prep_cmd_sock(SOCKET_URING_OP_SETSOCKOPT)
			probe.IsSupported(liburing.IORING_OP_FIXED_FD_INSTALL) // io_uring_prep_fixed_fd_install
	)
	if directAllocEnabled {
		if opt.RegisterFixedFiles < 1024 {
			opt.RegisterFixedFiles = 65535
		}
		if opt.RegisterFixedFiles > 65535 {
			soft, _, limitErr := sys.GetRLimit()
			if limitErr != nil {
				_ = ring.Close()
				err = NewRingErr(os.NewSyscallError("getrlimit", err))
				return
			}
			if soft < uint64(opt.RegisterFixedFiles) {
				_ = ring.Close()
				err = NewRingErr(errors.New("register fixed files too big, must smaller than " + strconv.FormatUint(soft, 10)))
				return
			}
		}
		if _, regErr := ring.RegisterFilesSparse(opt.RegisterFixedFiles); regErr != nil {
			_ = ring.Close()
			err = NewRingErr(regErr)
			return
		}
	}

	// sendZC
	var (
		sendZCEnabled    bool
		sendMSGZCEnabled bool
	)
	if opt.SendZCEnabled {
		sendZCEnabled = probe.IsSupported(liburing.IORING_OP_SEND_ZC)
		sendMSGZCEnabled = probe.IsSupported(liburing.IORING_OP_SENDMSG_ZC)
	}

	// multishotEnabledOps
	multishotEnabledOps := make(map[uint8]struct{})
	if liburing.VersionEnable(5, 19, 0) {
		multishotEnabledOps[liburing.IORING_OP_ACCEPT] = struct{}{}
	}
	if liburing.VersionEnable(6, 0, 0) {
		multishotEnabledOps[liburing.IORING_OP_RECV] = struct{}{}
		multishotEnabledOps[liburing.IORING_OP_RECVMSG] = struct{}{}
	}

	// producer
	producer := newOperationProducer(ring, opt.ProducerLockOSThread, int(opt.ProducerBatchSize), opt.ProducerBatchTimeWindow, opt.ProducerBatchIdleTime)
	// consumer
	consumer := newOperationConsumer(ring, opt.ConsumeBatchTimeCurve)
	// heartbeat
	hb := newHeartbeat(opt.HeartbeatTimeout, producer)

	// vortex
	v = &Vortex{
		ring:                ring,
		probe:               probe,
		sendZCEnabled:       sendZCEnabled,
		sendMSGZCEnabled:    sendMSGZCEnabled,
		producer:            producer,
		consumer:            consumer,
		heartbeat:           hb,
		directAllocEnabled:  directAllocEnabled,
		bufferAndRings:      brs,
		multishotEnabledOps: multishotEnabledOps,
		operations: sync.Pool{
			New: func() interface{} {
				return &Operation{
					code:     liburing.IORING_OP_LAST,
					flags:    borrowed,
					resultCh: make(chan Result, 2),
				}
			},
		},
		msgs: sync.Pool{
			New: func() interface{} {
				return &syscall.Msghdr{}
			},
		},
		timers: sync.Pool{
			New: func() interface{} {
				return time.NewTimer(0)
			},
		},
	}
	return
}

type Vortex struct {
	ring                *liburing.Ring
	probe               *liburing.Probe
	sendZCEnabled       bool
	sendMSGZCEnabled    bool
	producer            *operationProducer
	consumer            *operationConsumer
	heartbeat           *heartbeat
	directAllocEnabled  bool
	bufferAndRings      *BufferAndRings
	multishotEnabledOps map[uint8]struct{}
	operations          sync.Pool
	msgs                sync.Pool
	timers              sync.Pool
}

func (vortex *Vortex) Fd() int {
	return vortex.ring.Fd()
}

func (vortex *Vortex) fixedFdInstall(directFd int) (regularFd int, err error) {
	op := vortex.acquireOperation()
	op.PrepareFixedFdInstall(directFd)
	regularFd, _, err = vortex.submitAndWait(op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) cancelOperation(target *Operation) (err error) {
	if target.cancelAble() {
		op := vortex.acquireOperation()
		op.PrepareCancel(target)
		_, _, err = vortex.submitAndWait(op)
		vortex.releaseOperation(op)
	}
	return
}

func (vortex *Vortex) Close() (err error) {
	// heartbeat
	_ = vortex.heartbeat.Close()
	// close producer
	_ = vortex.producer.Close()
	// close consumer
	_ = vortex.consumer.Close()
	// unregister buffer and rings
	_ = vortex.bufferAndRings.Close()
	// unregister files
	if vortex.directAllocEnabled {
		ring := vortex.ring
		done := make(chan struct{})
		go func(ring *liburing.Ring, done chan struct{}) {
			_, _ = vortex.ring.UnregisterFiles()
			close(done)
		}(ring, done)
		timer := vortex.acquireTimer(50 * time.Millisecond)
		select {
		case <-done:
			break
		case <-timer.C:
			break
		}
		vortex.releaseTimer(timer)
	}
	// close ring
	err = vortex.ring.Close()
	return
}

func (vortex *Vortex) submit(op *Operation) bool {
	return vortex.producer.Produce(op)
}

func (vortex *Vortex) submitAndWait(op *Operation) (n int, cqeFlags uint32, err error) {
	if op.timeout != nil { // prepare timeout op
		timeoutOp := vortex.acquireOperation()
		defer vortex.releaseOperation(timeoutOp)
		timeoutOp.prepareLinkTimeout(op)
	}
RETRY:
	if ok := vortex.submit(op); ok {
		n, cqeFlags, err = vortex.awaitOperation(op)
		if err != nil {
			if errors.Is(err, ErrIOURingSQBusy) { // means cannot get sqe
				goto RETRY
			}
			if errors.Is(err, syscall.ECANCELED) && op.timeout != nil && op.addr2 != nil {
				timeoutOp := (*Operation)(op.addr2)
				if !timeoutOp.completed() {
					if timeoutErr := vortex.awaitTimeoutOp(timeoutOp); timeoutErr != nil {
						if errors.Is(timeoutErr, ErrTimeout) {
							err = ErrTimeout
						}
					}
				}
			}
			return
		}
		if op.timeout != nil && op.addr2 != nil { // await timeout
			op.timeout = nil // clean timeout for CQE_F_MORE, such as sendzc
			timeoutOp := (*Operation)(op.addr2)
			_ = vortex.awaitTimeoutOp(timeoutOp)
		}
		return
	}
	err = ErrCanceled
	return
}

func (vortex *Vortex) awaitOperation(op *Operation) (n int, cqeFlags uint32, err error) {
	r, ok := <-op.resultCh
	if !ok {
		op.Close()
		err = ErrCanceled
		return
	}
	n, cqeFlags, err = r.N, r.Flags, r.Err

	if err != nil {
		if errors.Is(err, syscall.ECANCELED) {
			err = ErrCanceled
		}
	}
	return
}

func (vortex *Vortex) awaitTimeoutOp(timeoutOp *Operation) (err error) {
	r, ok := <-timeoutOp.resultCh
	if ok {
		if r.Err != nil && errors.Is(r.Err, syscall.ETIME) {
			err = ErrTimeout
		}
	}
	return
}
