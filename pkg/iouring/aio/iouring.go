//go:build linux

package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/semaphores"
	"os"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

func createRing(id uint32, options Options) (r *URing, err error) {
	// ring
	rops := make([]iouring.Option, 0, 1)
	rops = append(rops, iouring.WithEntries(options.Entries), iouring.WithFlags(options.Flags))
	if options.Flags&iouring.SetupSQPoll != 0 {
		idle := options.SQThreadIdle
		if idle == 0 {
			idle = 15000
		}
		rops = append(rops, iouring.WithSQThreadIdle(idle))
	}
	if options.Flags&iouring.SetupSQAff != 0 {
		rops = append(rops, iouring.WithSQThreadCPU(id))
	}

	ring, ringErr := iouring.New(rops...)
	if ringErr != nil {
		err = ringErr
		return
	}
	// submit semaphores
	prepareIdleTime := options.PrepSQEIdleTime
	if prepareIdleTime < 1 {
		prepareIdleTime = defaultPrepareSQIdleTime
	}
	submitSemaphores, semaphoresErr := semaphores.New(prepareIdleTime)
	if semaphoresErr != nil {
		err = semaphoresErr
		return
	}
	// register buffers
	var (
		bufferRegistered = false
		buffers          *Queue[FixedBuffer]
	)
	if size, count := options.RegisterFixedBufferSize, options.RegisterFixedBufferCount; count > 0 && size > 0 {
		buffers = NewQueue[FixedBuffer]()
		iovecs := make([]syscall.Iovec, count)
		for i := uint32(0); i < count; i++ {
			buf := make([]byte, size)
			buffers.Enqueue(&FixedBuffer{
				value: buf,
				src:   id,
				index: int(i),
			})
			iovecs[i] = syscall.Iovec{
				Base: &buf[0],
				Len:  uint64(size),
			}
		}
		_, regErr := ring.RegisterBuffers(iovecs)
		if regErr != nil {
			options.RegisterFixedBufferSize, options.RegisterFixedBufferCount = 0, 0
			for i := uint32(0); i < count; i++ {
				_ = buffers.Dequeue()
			}
			buffers = nil
		} else {
			bufferRegistered = true
		}
	}

	r = &URing{
		id:               id,
		ring:             ring,
		prepSQEBatchSize: options.PrepSQEBatchSize,
		waitCQEBatchSize: options.WaitCQEBatchSize,
		waitCQETimeCurve: options.WaitCQETimeCurve,
		wg:               sync.WaitGroup{},
		bufferRegistered: bufferRegistered,
		buffers:          buffers,
		queue:            NewQueue[Operation](),
		submitSemaphores: submitSemaphores,
	}

	return
}

type URing struct {
	id               uint32
	cancel           context.CancelFunc
	ring             *iouring.Ring
	prepSQEBatchSize uint32
	waitCQEBatchSize uint32
	waitCQETimeCurve Curve
	wg               sync.WaitGroup
	bufferRegistered bool
	buffers          *Queue[FixedBuffer]
	queue            *Queue[Operation]
	submitSemaphores *semaphores.Semaphores
}

func (r *URing) AcquireBuffer() *FixedBuffer {
	if r.bufferRegistered {
		return r.buffers.Dequeue()
	}
	return nil
}

func (r *URing) ReleaseBuffer(buf *FixedBuffer) {
	if buf == nil {
		return
	}
	if r.bufferRegistered {
		buf.Reset()
		r.buffers.Enqueue(buf)
	}
}

func (r *URing) Serve(ctx context.Context) {
	r.wg.Add(2)
	ctx, r.cancel = context.WithCancel(ctx)
	go r.preparingSQE(ctx)
	go r.waitingCQE(ctx)
}

func (r *URing) Close() (err error) {
	r.cancel()
	r.wg.Wait()
	ring := r.ring
	if r.bufferRegistered {
		_, _ = ring.UnregisterBuffers()
		for {
			if buf := r.buffers.Dequeue(); buf == nil {
				break
			}
		}
	}
	err = ring.Close()
	return
}

func (r *URing) preparingSQE(ctx context.Context) {
	defer r.wg.Done()

	// lock os thread
	//runtime.LockOSThread()
	//defer runtime.UnlockOSThread()

	// cpu affinity
	//_ = process.SetCPUAffinity(int(vortex.options.PrepareSQEAffinityCPU))

	ring := r.ring

	queue := r.queue
	shs := r.submitSemaphores
	var (
		peeked       uint32
		prepared     uint32
		needToSubmit int64
	)

	prepareBatch := r.prepSQEBatchSize
	if prepareBatch < 1 {
		prepareBatch = 1024
	}
	operations := make([]*Operation, prepareBatch)
	for {
		if peeked = queue.PeekBatch(operations); peeked == 0 {
			if needToSubmit > 0 {
				submitted, _ := ring.Submit()
				needToSubmit -= int64(submitted)
			}
			if waitErr := shs.Wait(ctx); waitErr != nil {
				if errors.Is(waitErr, context.Canceled) {
					break
				}
			}
			continue
		}
		for i := uint32(0); i < peeked; i++ {
			op := operations[i]
			if op == nil {
				continue
			}
			operations[i] = nil
			if op.status.CompareAndSwap(ReadyOperationStatus, ProcessingOperationStatus) {
				if prepErr := r.prepareSQE(op); prepErr != nil { // when prep err occur, means invalid op kind or no sqe left
					op.setResult(0, prepErr)
					if errors.Is(prepErr, syscall.EBUSY) { // no sqe left
						break
					}
					prepared++ // prepareSQE nop whit out userdata, so prepared++
					continue
				}
				prepared++
			}
		}
		// submit
		if prepared > 0 || needToSubmit > 0 {
			submitted, _ := ring.Submit()
			needToSubmit += int64(prepared) - int64(submitted)
			queue.Advance(prepared)
			prepared = 0
		}
	}
	// evict
	if remains := queue.Length(); remains > 0 {
		peeked = queue.PeekBatch(operations)
		for i := uint32(0); i < peeked; i++ {
			op := operations[i]
			operations[i] = nil
			op.setResult(0, Uncompleted)
		}
	}
	// prepare noop to wakeup cq waiter
	for {
		sqe := ring.GetSQE()
		if sqe == nil {
			time.Sleep(1 * time.Millisecond)
			continue
		}
		sqe.PrepareNop()
		for {
			if _, subErr := ring.Submit(); subErr != nil {
				time.Sleep(1 * time.Millisecond)
				continue
			}
			break
		}
		break
	}
	return
}

func (r *URing) waitingCQE(ctx context.Context) {
	defer r.wg.Done()

	// lock os thread
	//runtime.LockOSThread()
	//defer runtime.UnlockOSThread()

	// cpu affinity
	//_ = process.SetCPUAffinity(int(vortex.options.WaitCQEAffinityCPU))

	ring := r.ring
	transmission := NewCurveTransmission(r.waitCQETimeCurve)
	cqeWaitMaxCount, cqeWaitTimeout := transmission.Up()
	waitCQBatchSize := r.waitCQEBatchSize
	if waitCQBatchSize < 1 {
		waitCQBatchSize = 1024
	}
	cq := make([]*iouring.CompletionQueueEvent, waitCQBatchSize)
	stopped := false
	for {
		select {
		case <-ctx.Done():
			stopped = true
			break
		default:
			if completed := ring.PeekBatchCQE(cq); completed > 0 {
				for i := uint32(0); i < completed; i++ {
					cqe := cq[i]
					cq[i] = nil
					if cqe.UserData == 0 { // no userdata means no op
						continue
					}
					if notify := cqe.Flags&iouring.CQEFNotify != 0; notify { // used by send zc
						continue
					}
					// get op from
					copPtr := cqe.GetData()
					cop := (*Operation)(copPtr)
					// handle
					if cop.status.CompareAndSwap(ProcessingOperationStatus, CompletedOperationStatus) { // not done
						var (
							opN   int
							opErr error
						)
						if cqe.Res < 0 {
							opErr = os.NewSyscallError(cop.Name(), syscall.Errno(-cqe.Res))
						} else {
							opN = int(cqe.Res)
						}
						cop.setResult(opN, opErr)
					}
				}
				// CQAdvance
				ring.CQAdvance(completed)
			} else {
				if _, waitErr := ring.WaitCQEs(cqeWaitMaxCount, cqeWaitTimeout, nil); waitErr != nil {
					cqeWaitMaxCount, cqeWaitTimeout = transmission.Down()
				} else {
					cqeWaitMaxCount, cqeWaitTimeout = transmission.Up()
				}
			}
			break
		}

		if stopped {
			break
		}
	}

	return
}

func (r *URing) prepareSQE(op *Operation) error {
	sqe := r.ring.GetSQE()
	if sqe == nil {
		return os.NewSyscallError("ring_getsqe", syscall.EBUSY)
	}
	switch op.kind {
	case iouring.OpNop:
		sqe.PrepareNop()
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpConnect:
		addrPtr := (*syscall.RawSockaddrAny)(unsafe.Pointer(op.msg.Name))
		addrLenPtr := uint64(op.msg.Namelen)
		sqe.PrepareConnect(op.fd, addrPtr, addrLenPtr)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpAccept:
		addrPtr := (*syscall.RawSockaddrAny)(unsafe.Pointer(op.msg.Name))
		addrLenPtr := uint64(uintptr(unsafe.Pointer(&op.msg.Namelen)))
		sqe.PrepareAccept(op.fd, addrPtr, addrLenPtr, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpClose:
		sqe.PrepareClose(op.fd)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpReadFixed:
		b := uintptr(unsafe.Pointer(op.msg.Name))
		bLen := op.msg.Namelen
		idx := op.msg.Iovlen
		sqe.PrepareReadFixed(op.fd, b, bLen, 0, int(idx))
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpWriteFixed:
		b := uintptr(unsafe.Pointer(op.msg.Name))
		bLen := op.msg.Namelen
		idx := op.msg.Iovlen
		sqe.PrepareWriteFixed(op.fd, b, bLen, 0, int(idx))
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpRecv:
		b := uintptr(unsafe.Pointer(op.msg.Name))
		bLen := op.msg.Namelen
		sqe.PrepareRecv(op.fd, b, bLen, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpSend:
		b := uintptr(unsafe.Pointer(op.msg.Name))
		bLen := op.msg.Namelen
		sqe.PrepareSend(op.fd, b, bLen, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpSendZC:
		b := uintptr(unsafe.Pointer(op.msg.Name))
		bLen := op.msg.Namelen
		sqe.PrepareSendZC(op.fd, b, bLen, 0, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpRecvmsg:
		sqe.PrepareRecvMsg(op.fd, &op.msg, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpSendmsg:
		sqe.PrepareSendMsg(op.fd, &op.msg, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpSendMsgZC:
		sqe.PrepareSendmsgZC(op.fd, &op.msg, 0)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpSplice:
		sqe.PrepareSplice(op.pipe.fdIn, op.pipe.offIn, op.pipe.fdOut, op.pipe.offOut, op.pipe.nbytes, op.pipe.spliceFlags)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpTee:
		sqe.PrepareTee(op.pipe.fdIn, op.pipe.fdOut, op.pipe.nbytes, op.pipe.spliceFlags)
		sqe.SetData(unsafe.Pointer(op))
		break
	case iouring.OpAsyncCancel:
		sqe.PrepareCancel(uintptr(op.ptr), 0)
		break
	default:
		sqe.PrepareNop()
		return UnsupportedOp
	}
	runtime.KeepAlive(sqe)
	return nil
}
