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
	"unsafe"
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

	// sendZC
	var (
		sendZC    bool
		sendMSGZC bool
	)
	if opt.SendZC {
		sendZC = probe.IsSupported(liburing.IORING_OP_SEND_ZC)
		sendMSGZC = probe.IsSupported(liburing.IORING_OP_SENDMSG_ZC)
	}
	// conn ring buffer config
	connRingBufferConfig := newRingBufferConfig(ring, opt.ConnRingBufferSize, opt.ConnRingBufferCount)

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
		ring:                 ring,
		probe:                probe,
		sendZC:               sendZC,
		sendMSGZC:            sendMSGZC,
		producer:             producer,
		consumer:             consumer,
		heartbeat:            hb,
		directAllocEnabled:   directAllocEnabled,
		connRingBufferConfig: connRingBufferConfig,
		multishotEnabledOps:  multishotEnabledOps,
		operations: sync.Pool{
			New: func() interface{} {
				return &Operation{
					code:     liburing.IORING_OP_LAST,
					flags:    borrowed,
					resultCh: make(chan Result, 1),
				}
			},
		},
		multishotOperations: sync.Pool{
			New: func() interface{} {
				return &Operation{
					code:     liburing.IORING_OP_LAST,
					flags:    borrowed,
					resultCh: make(chan Result, connRingBufferConfig.count),
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
	ring                 *liburing.Ring
	probe                *liburing.Probe
	sendZC               bool
	sendMSGZC            bool
	producer             *operationProducer
	consumer             *operationConsumer
	heartbeat            *heartbeat
	directAllocEnabled   bool
	connRingBufferConfig *RingBufferConfig
	multishotEnabledOps  map[uint8]struct{}
	operations           sync.Pool
	multishotOperations  sync.Pool
	msgs                 sync.Pool
	timers               sync.Pool
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

func (vortex *Vortex) MultishotReceiveMSGEnabled() bool {
	return vortex.MultishotEnabled(liburing.IORING_OP_RECVMSG)
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
	// heartbeat
	_ = vortex.heartbeat.Close()
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

func (vortex *Vortex) acquireMultishotOperation() *Operation {
	op := vortex.multishotOperations.Get().(*Operation)
	return op
}

func (vortex *Vortex) releaseMultishotOperation(op *Operation) {
	if op.releaseAble() {
		op.reset()
		vortex.multishotOperations.Put(op)
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

const (
	defaultHeartbeatTimeout = 30 * time.Second
)

func newHeartbeat(timeout time.Duration, producer *operationProducer) *heartbeat {
	if timeout < 1 {
		timeout = defaultHeartbeatTimeout
	}
	hb := &heartbeat{
		producer: producer,
		ticker:   time.NewTicker(timeout),
		wg:       new(sync.WaitGroup),
		done:     make(chan struct{}),
	}
	go hb.start()
	return hb
}

type heartbeat struct {
	producer *operationProducer
	ticker   *time.Ticker
	wg       *sync.WaitGroup
	done     chan struct{}
}

func (hb *heartbeat) start() {
	hb.wg.Add(1)
	defer hb.wg.Done()

	op := &Operation{}
	_ = op.PrepareNop()
	for {
		select {
		case <-hb.done:
			hb.ticker.Stop()
			return
		case <-hb.ticker.C:
			hb.producer.Produce(op)
			break
		}
	}
}

func (hb *heartbeat) Close() error {
	close(hb.done)
	hb.wg.Wait()
	return nil
}
