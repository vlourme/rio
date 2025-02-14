package ring

import (
	"context"
	"fmt"
	"github.com/brickingsoft/errors"
	"github.com/brickingsoft/rio/pkg/sys"
	"github.com/pawelgaczynski/giouring"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

func New(size int) (*Ring, error) {
	if size <= 0 {
		size = 8 // todo
	}
	r, rErr := giouring.CreateRing(uint32(size))
	if rErr != nil {
		return nil, rErr
	}
	queue := NewOperationQueue(size)
	// check zc
	major, minor := sys.KernelVersion()
	if major >= 6 && minor >= 0 {
		sendZCEnable = true
	}
	if major >= 6 && minor >= 1 {
		sendMsgZCEnable = true
	}
	// wait timeout
	waitTimeout := syscall.NsecToTimespec((50 * time.Millisecond).Nanoseconds()) // todo wait timeout
	return &Ring{
		ring:        r,
		queue:       queue,
		waitTimeout: waitTimeout,
		operations: sync.Pool{
			New: func() interface{} {
				return &Operation{
					ch: make(chan Result, 1),
				}
			},
		},
		operationRCH: make(chan *Operation, size),
		stopCh:       nil,
		wg:           sync.WaitGroup{},
	}, nil
}

type Ring struct {
	ring         *giouring.Ring
	queue        *OperationQueue
	waitTimeout  syscall.Timespec
	operations   sync.Pool
	operationRCH chan *Operation
	stopCh       chan struct{}
	wg           sync.WaitGroup
}

func (ring *Ring) AcquireOperation() *Operation {
	op := ring.operations.Get().(*Operation)
	return op
}

func (ring *Ring) ReleaseOperation(op *Operation) {
	if op.reset() {
		ring.operations.Put(op)
	} else {
		ring.operationRCH <- op
	}
}

func (ring *Ring) Push(op *Operation) error {
	if ring.queue.Enqueue(op) {
		return nil
	}
	return errors.New("failed to push operation, queue is full") // todo make err
}

func (ring *Ring) Start(ctx context.Context) {
	ring.stopCh = make(chan struct{}, 1)
	ring.releasingOp(ctx)
	ring.wg.Add(1)
	go func(ctx context.Context, ring *Ring) {
		defer ring.wg.Done()

		prepared := make([]*Operation, ring.queue.capacity)
		cqes := make([]*giouring.CompletionQueueEvent, ring.queue.capacity)
		zeroPeeked := 0
		stopped := false
		for {
			select {
			case <-ctx.Done():
				stopped = true
				break
			case <-ring.stopCh:
				stopped = true
				break
			default:
				// submit
				submitted := ring.submit(prepared)
				ring.waitAndHandleCQEs(cqes)
				if submitted {
					continue
				}

				zeroPeeked++
				if zeroPeeked > 10 {
					runtime.Gosched()
				}
				time.Sleep(500 * time.Nanosecond)
				//time.Sleep(1 * time.Second)

				// peek
				//peeked := ring.queue.PeekBatch(prepared)
				//if peeked == 0 {
				//	ring.waitAndHandleCQEs(cqes)
				//
				//	zeroPeeked++
				//	if zeroPeeked > 10 {
				//		runtime.Gosched()
				//	}
				//	time.Sleep(500 * time.Nanosecond)
				//	//time.Sleep(1 * time.Second)
				//	continue
				//}
				//// prepare
				//set := int64(0)
				//for i := int64(0); i < peeked; i++ {
				//	op := prepared[i]
				//	if op == nil {
				//		break
				//	}
				//	prepared[i] = nil
				//	sqe := ring.prepare(op)
				//	runtime.KeepAlive(op)
				//	if sqe == nil {
				//		break
				//	}
				//	set++
				//}
				//if set == 0 {
				//	continue
				//}
				//// submit
				//for {
				//	_, submitErr := ring.ring.Submit()
				//	if submitErr != nil {
				//		if errors.Is(submitErr, syscall.EAGAIN) || errors.Is(submitErr, syscall.EINTR) || errors.Is(submitErr, syscall.ETIME) {
				//			continue
				//		}
				//		break
				//	}
				//	ring.queue.Advance(set)
				//	break
				//}
				//// wait cqe
				//ring.waitAndHandleCQEs(cqes)
				//for {
				//	_, waitErr := ring.ring.WaitCQEs(1, &waitTimeout, nil)
				//	if waitErr != nil {
				//		if errors.Is(waitErr, syscall.EAGAIN) {
				//			continue
				//		}
				//		cqReady = false
				//		break
				//	}
				//	cqReady = true
				//	break
				//}
				//if !cqReady {
				//	continue
				//}
				//// peek cqe
				//completed := ring.ring.PeekBatchCQE(cqes)
				//fmt.Println("completed", completed)
				//if completed == 0 {
				//	continue
				//}
				//for i := uint32(0); i < completed; i++ {
				//	cqe := cqes[i]
				//	if cqe.UserData == 0 {
				//		continue
				//	}
				//
				//	cop := (*Operation)(unsafe.Pointer(uintptr(cqe.UserData)))
				//	if cop.done.CompareAndSwap(false, true) {
				//		fmt.Println("done: flags", cqe.Flags&giouring.CQEFMore, cqe.Flags&giouring.CQEFNotif)
				//		// sent result when op not done (when done means timeout or ctx canceled)
				//		var res int
				//		var err error
				//		if cqe.Res < 0 {
				//			err = syscall.Errno(-cqe.Res)
				//			// release hijacked when err occur
				//			cop.hijacked.Store(false)
				//		} else {
				//			res = int(cqe.Res)
				//		}
				//		cop.ch <- Result{
				//			N:   res,
				//			Err: err,
				//		}
				//	} else {
				//		fmt.Println("done:", cop.hijacked.Load(), string(cop.hijackedBytes))
				//		// handle done but hijacked
				//		// 1. by timeout or ctx canceled, so should be hijacked
				//		// 2. by send_zc or sendmsg_zc, so should be hijacked
				//		if cop.hijacked.CompareAndSwap(true, false) {
				//			cop.ch <- Result{}
				//		}
				//	}
				//}
				//if completed > 0 {
				//	ring.ring.CQAdvance(completed)
				//}
			}
			if stopped {
				break
			}
		}
		// send failed for remains
		if remains := ring.queue.Len(); remains > 0 {
			peeked := ring.queue.PeekBatch(prepared)
			for i := int64(0); i < peeked; i++ {
				op := prepared[i]
				prepared[i] = nil
				op.ch <- Result{
					N:   0,
					Err: errors.New("uncompleted via closed"), // todo make err
				}
			}
		}
		fmt.Println("closed all pending operations")
		// handle cached
		close(ring.operationRCH)
		// queue exit
		ring.ring.QueueExit()
	}(ctx, ring)
}

func (ring *Ring) submit(prepared []*Operation) bool {
	peeked := ring.queue.PeekBatch(prepared)
	if peeked == 0 {
		return false
	}
	set := int64(0)
	for i := int64(0); i < peeked; i++ {
		op := prepared[i]
		if op == nil {
			break
		}
		prepared[i] = nil
		sqe := ring.prepare(op)
		runtime.KeepAlive(op)
		if sqe == nil {
			break
		}
		set++
	}
	if set == 0 {
		return false
	}
	// submit
	for {
		_, submitErr := ring.ring.Submit()
		if submitErr != nil {
			if errors.Is(submitErr, syscall.EAGAIN) || errors.Is(submitErr, syscall.EINTR) || errors.Is(submitErr, syscall.ETIME) {
				continue
			}
			break
		}
		// adv queue
		ring.queue.Advance(set)
		return true
	}
	return false
}

func (ring *Ring) waitAndHandleCQEs(cqes []*giouring.CompletionQueueEvent) {
	// wait cqe
	cqReady := false
	for {
		_, waitErr := ring.ring.WaitCQEs(1, &ring.waitTimeout, nil)
		if waitErr != nil {
			if errors.Is(waitErr, syscall.EAGAIN) {
				continue
			}
			cqReady = false
			break
		}
		cqReady = true
		break
	}
	if !cqReady {
		return
	}

	completed := ring.ring.PeekBatchCQE(cqes)
	fmt.Println("completed", completed)
	if completed == 0 {
		return
	}
	for i := uint32(0); i < completed; i++ {
		cqe := cqes[i]
		if cqe.UserData == 0 {
			continue
		}

		cop := (*Operation)(unsafe.Pointer(uintptr(cqe.UserData)))
		if cop.done.CompareAndSwap(false, true) {
			fmt.Println("done: flags", cqe.Flags&giouring.CQEFMore, cqe.Flags&giouring.CQEFNotif)
			// sent result when op not done (when done means timeout or ctx canceled)
			var res int
			var err error
			if cqe.Res < 0 {
				err = syscall.Errno(-cqe.Res)
				// release hijacked when err occur
				cop.hijacked.Store(false)
			} else {
				res = int(cqe.Res)
			}
			cop.ch <- Result{
				N:   res,
				Err: err,
			}
		} else {
			fmt.Println("done:", cop.hijacked.Load(), string(cop.hijackedBytes))
			// handle done but hijacked
			// 1. by timeout or ctx canceled, so should be hijacked
			// 2. by send_zc or sendmsg_zc, so should be hijacked
			if cop.hijacked.CompareAndSwap(true, false) {
				cop.ch <- Result{}
			}
		}
	}
	if completed > 0 {
		ring.ring.CQAdvance(completed)
	}

	return
}

func (ring *Ring) releasingOp(ctx context.Context) {
	// todo batch
	ring.wg.Add(1)
	go func(ctx context.Context, ring *Ring) {
		stopped := false
		for {
			select {
			case <-ctx.Done():
				stopped = true
				break
			case <-ring.stopCh:
				stopped = true
				break
			case op, ok := <-ring.operationRCH:
				if !ok {
					stopped = true
					break
				}
				//fmt.Println("releasing operation", op.hijacked.Load(), string(op.hijackedBytes))
				<-op.ch
				ring.ReleaseOperation(op)
				break
			}
			if stopped {
				break
			}
		}
		//fmt.Println("releasing operation")
		ring.wg.Done()
	}(ctx, ring)
}

func (ring *Ring) Stop() {
	if ring.stopCh != nil {
		close(ring.stopCh)
		ring.wg.Wait()
		return
	}
	ring.ring.QueueExit()
	return
}

func (ring *Ring) prepare(op *Operation) (sqe *giouring.SubmissionQueueEntry) {
	sqe = ring.ring.GetSQE()
	if sqe == nil {
		return
	}
	switch op.kind {
	case nop:
		sqe.PrepareNop()
		break
	case acceptOp:
		addrPtr := uintptr(unsafe.Pointer(op.msg.Name))
		addrLenPtr := uint64(uintptr(unsafe.Pointer(&op.msg.Namelen)))
		sqe.PrepareAccept(op.fd, addrPtr, addrLenPtr, 0)
		break
	case receiveOp:
		b := uintptr(unsafe.Pointer(op.msg.Iov.Base))
		bLen := uint32(op.msg.Iov.Len)
		sqe.PrepareRecv(op.fd, b, bLen, 0)
		break
	case sendOp:
		b := uintptr(unsafe.Pointer(op.msg.Iov.Base))
		bLen := uint32(op.msg.Iov.Len)
		sqe.PrepareSend(op.fd, b, bLen, 0)
		break
	case sendZCOp:
		b := unsafe.Slice(op.msg.Iov.Base, op.msg.Iov.Len)
		sqe.PrepareSendZC(op.fd, b, 0, 0)
		break
	case receiveFromOp, receiveMsgOp:
		msg := op.msg
		sqe.PrepareRecvMsg(op.fd, &msg, 0)
		break
	case sendToOp, sendMsgOp:
		msg := op.msg
		sqe.PrepareSendMsg(op.fd, &msg, 0)
		break
	case sendMsgZcOp:
		msg := op.msg
		sqe.PrepareSendmsgZC(op.fd, &msg, 0)
		break
	case spliceOp:
		sp := op.splice
		sqe.PrepareSplice(sp.fdIn, sp.offIn, sp.fdOut, sp.offOut, sp.nbytes, sp.spliceFlags)
		break
	case teeOp:
		sp := op.splice
		sqe.PrepareTee(sp.fdIn, sp.fdOut, sp.nbytes, sp.spliceFlags)
		break
	default:
		sqe.PrepareNop()
		break
	}
	sqe.SetData(unsafe.Pointer(op))
	runtime.KeepAlive(sqe)
	return
}
