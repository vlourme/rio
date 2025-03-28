//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

func Open(options ...Option) (v *Vortex, err error) {
	// version check
	if !liburing.VersionEnable(5, 13, 0) { // support recv_send
		err = NewRingErr(errors.New("kernel version must >= 5.13"))
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
	// register files
	var (
		directAllocEnabled = !liburing.VersionMatchFlavor(opt.DisableDirectAllocFeatKernelFlavorBlackList) && // disabled by black list
			liburing.VersionEnable(6, 7, 0) && // support io_uring_prep_cmd_sock(SOCKET_URING_OP_SETSOCKOPT)
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

	// producer
	var (
		producerLockOSThread    = opt.SQEProducerLockOSThread
		producerBatchSize       = opt.SQEProducerBatchSize
		producerBatchTimeWindow = opt.SQEProducerBatchTimeWindow
		producerBatchIdleTime   = opt.SQEProducerBatchIdleTime
	)
	if ring.Flags()&liburing.IORING_SETUP_SINGLE_ISSUER != 0 {
		producerLockOSThread = true
	}
	producer := newSQEChanProducer(ring, producerLockOSThread, int(producerBatchSize), producerBatchTimeWindow, producerBatchIdleTime)
	// consumer
	consumerType := strings.ToUpper(strings.TrimSpace(opt.CQEConsumerType))
	var consumer OperationConsumer
	switch consumerType {
	case CQEConsumerPushType:
		consumer, err = newPushTypedOperationConsumer(ring)
		break
	default:
		consumer, err = newPullTypedOperationConsumer(ring, opt.CQEPullTypedConsumeIdleTime, opt.CQEPullTypedConsumeTimeCurve)
		break
	}
	if err != nil {
		_ = producer.Close()
		_ = ring.Close()
		err = NewRingErr(err)
		return
	}
	// heartbeat
	heartbeatTimeout := opt.HeartbeatTimeout
	if heartbeatTimeout < 1 {
		heartbeatTimeout = defaultHeartbeatTimeout
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
	// sendZC
	var (
		sendZC    bool
		sendMSGZC bool
	)
	if opt.SendZC {
		sendZC = probe.IsSupported(liburing.IORING_OP_SEND_ZC)
		sendMSGZC = probe.IsSupported(liburing.IORING_OP_SENDMSG_ZC)
	}

	// vortex
	v = &Vortex{
		ring:                ring,
		probe:               probe,
		heartbeatTimeout:    heartbeatTimeout,
		sendZC:              sendZC,
		sendMSGZC:           sendMSGZC,
		done:                make(chan struct{}),
		producer:            producer,
		consumer:            consumer,
		directAllocEnabled:  directAllocEnabled,
		multishotEnabledOps: multishotEnabledOps,
		operations: sync.Pool{
			New: func() interface{} {
				return &Operation{
					code:     liburing.IORING_OP_LAST,
					flags:    borrowed,
					resultCh: make(chan Result, 1),
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
	// heartbeat
	go v.heartbeat()
	return
}

type Vortex struct {
	ring                *liburing.Ring
	probe               *liburing.Probe
	heartbeatTimeout    time.Duration
	sendZC              bool
	sendMSGZC           bool
	done                chan struct{}
	producer            SQEProducer
	consumer            OperationConsumer
	directAllocEnabled  bool
	multishotEnabledOps map[uint8]struct{}
	operations          sync.Pool
	msgs                sync.Pool
	timers              sync.Pool
}

func (vortex *Vortex) Fd() int {
	return vortex.ring.Fd()
}

func (vortex *Vortex) DirectAllocEnabled() bool {
	return vortex.directAllocEnabled
}

func (vortex *Vortex) SendZCEnabled() bool {
	return vortex.sendZC
}

func (vortex *Vortex) SendMSGZCEnabled() bool {
	return vortex.sendMSGZC
}

func (vortex *Vortex) MultishotEnabled(op uint8) bool {
	_, has := vortex.multishotEnabledOps[op]
	return has
}

func (vortex *Vortex) MultishotAcceptEnabled() bool {
	return vortex.MultishotEnabled(liburing.IORING_OP_ACCEPT)
}

func (vortex *Vortex) MultishotReceiveEnabled() bool {
	return vortex.MultishotEnabled(liburing.IORING_OP_RECV)
}

func (vortex *Vortex) OpSupported(op uint8) bool {
	return vortex.probe.IsSupported(op)
}

func (vortex *Vortex) Submit(op *Operation) bool {
	return vortex.producer.Produce(op)
}

func (vortex *Vortex) FixedFdInstall(directFd int) (regularFd int, err error) {
	op := vortex.acquireOperation()
	op.PrepareFixedFdInstall(directFd)
	regularFd, _, err = vortex.submitAndWait(op)
	vortex.releaseOperation(op)
	return
}

func (vortex *Vortex) CancelOperation(target *Operation) (err error) {
	if target.cancelAble() {
		op := vortex.acquireOperation()
		op.PrepareCancel(target)
		_, _, err = vortex.submitAndWait(op)
		vortex.releaseOperation(op)
		if err != nil && !IsOperationInvalid(err) { // discard target missing
			err = nil
		}
	}
	return
}

func (vortex *Vortex) Close() (err error) {
	// done
	close(vortex.done)
	// close producer
	_ = vortex.producer.Close()
	// close consumer
	_ = vortex.consumer.Close()

	// unregister files
	if vortex.DirectAllocEnabled() {
		_, _ = vortex.ring.UnregisterFiles()
	}
	// close
	err = vortex.ring.Close()
	return
}

func (vortex *Vortex) heartbeat() {
	ticker := time.NewTicker(vortex.heartbeatTimeout)
	defer ticker.Stop()
	for {
		select {
		case <-vortex.done:
			return
		case <-ticker.C:
			op := &Operation{}
			_ = op.PrepareNop()
			if ok := vortex.Submit(op); !ok {
				break
			}
			break
		}
	}
}

func (vortex *Vortex) acquireOperation() *Operation {
	op := vortex.operations.Get().(*Operation)
	return op
}

func (vortex *Vortex) releaseOperation(op *Operation) {
	if op.releaseAble() {
		op.reset()
		vortex.operations.Put(op)
	}
}

func (vortex *Vortex) acquireTimer(timeout time.Duration) *time.Timer {
	timer := vortex.timers.Get().(*time.Timer)
	timer.Reset(timeout)
	return timer
}

func (vortex *Vortex) releaseTimer(timer *time.Timer) {
	timer.Stop()
	vortex.timers.Put(timer)
}

func (vortex *Vortex) acquireMsg(b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32) *syscall.Msghdr {
	msg := vortex.msgs.Get().(*syscall.Msghdr)
	bLen := len(b)
	if bLen > 0 {
		msg.Iov = &syscall.Iovec{
			Base: &b[0],
			Len:  uint64(bLen),
		}
		msg.Iovlen = 1
	}
	oobLen := len(oob)
	if oobLen > 0 {
		msg.Control = &oob[0]
		msg.SetControllen(oobLen)
	}
	if addr != nil {
		msg.Name = (*byte)(unsafe.Pointer(addr))
		msg.Namelen = uint32(addrLen)
	}
	msg.Flags = flags
	return msg
}

func (vortex *Vortex) releaseMsg(msg *syscall.Msghdr) {
	msg.Name = nil
	msg.Namelen = 0
	msg.Iov = nil
	msg.Iovlen = 0
	msg.Control = nil
	msg.Controllen = 0
	msg.Flags = 0
	vortex.msgs.Put(msg)
	return
}

func (vortex *Vortex) submitAndWait(op *Operation) (n int, cqeFlags uint32, err error) {
	if op.timeout != nil { // prepare timeout op
		timeoutOp := vortex.acquireOperation()
		defer vortex.releaseOperation(timeoutOp)
		timeoutOp.prepareLinkTimeout(op)
	}
RETRY:
	if ok := vortex.Submit(op); ok {
		n, cqeFlags, err = vortex.awaitOperation(op)
		if err != nil {
			if errors.Is(err, ErrIOURingSQBusy) { // means cannot get sqe
				goto RETRY
			}
			if errors.Is(err, syscall.ECANCELED) && op.timeout != nil && op.addr2 != nil {
				op.timeout = nil // clean timeout for CQE_F_MORE, such as sendzc
				timeoutOp := (*Operation)(op.addr2)
				if timeoutErr := vortex.awaitTimeoutOp(timeoutOp); timeoutErr != nil {
					if errors.Is(timeoutErr, ErrTimeout) {
						err = ErrTimeout
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
		} else {
			err = os.NewSyscallError(op.Name(), err)
		}
	}
	return
}

func (vortex *Vortex) awaitOperationWithDeadline(op *Operation, deadline time.Time) (n int, cqeFlags uint32, err error) {
	if deadline.IsZero() {
		n, cqeFlags, err = vortex.awaitOperation(op)
		return
	}
	timeout := time.Until(deadline)
	timer := vortex.acquireTimer(timeout)
	defer vortex.releaseTimer(timer)

	select {
	case r, ok := <-op.resultCh:
		if !ok {
			op.Close()
			err = ErrCanceled
			return
		}
		n, cqeFlags, err = r.N, r.Flags, r.Err
		if err != nil {
			if errors.Is(err, syscall.ECANCELED) {
				err = ErrCanceled
			} else {
				err = os.NewSyscallError(op.Name(), err)
			}
		}
		break
	case <-timer.C:
		err = ErrTimeout
		break
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
