package ring

import (
	"context"
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
	waitTimeout := 50 * time.Millisecond // todo wait timeout opt
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
		stopCh: nil,
		wg:     sync.WaitGroup{},
	}, nil
}

type Ring struct {
	ring        *giouring.Ring
	queue       *OperationQueue
	waitTimeout time.Duration
	operations  sync.Pool
	stopCh      chan struct{}
	wg          sync.WaitGroup
}

func (ring *Ring) AcquireOperation() *Operation {
	op := ring.operations.Get().(*Operation)
	return op
}

func (ring *Ring) ReleaseOperation(op *Operation) {
	if op.reset() {
		ring.operations.Put(op)
	}
}

func (ring *Ring) CancelOperation(op *Operation) {
	op.kind = cancelOp
	for {
		if err := ring.Push(op); err != nil {
			continue
		}
		break
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
	ring.listenSQ(ctx)
	ring.listenCQ(ctx)
}

func (ring *Ring) listenSQ(ctx context.Context) {
	ring.wg.Add(1)
	go func(ctx context.Context, ring *Ring) {
		stopCh := ring.stopCh
		queue := ring.queue
		ready := make([]*Operation, queue.capacity) // todo batch opt
		peekNothingTimes := 0
		stopped := false
		for {
			select {
			case <-ctx.Done():
				stopped = true
				break
			case <-stopCh:
				stopped = true
				break
			default:
				peeked := queue.PeekBatch(ready)
				if peeked == 0 {
					peekNothingTimes++
					if peekNothingTimes > 10 {
						peekNothingTimes = 0
						runtime.Gosched()
					} else {
						time.Sleep(500 * time.Nanosecond)
					}
					break
				}
				prepared := int64(0)
				for i := int64(0); i < peeked; i++ {
					op := ready[i]
					if op == nil {
						break
					}
					ready[i] = nil
					sqe := ring.prepare(op)
					runtime.KeepAlive(op)
					if sqe == nil {
						break
					}
					prepared++
				}
				if prepared == 0 {
					break
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
					ring.queue.Advance(prepared)
					break
				}
			}
			if stopped {
				break
			}
		}
		// evict remain
		if remains := ring.queue.Len(); remains > 0 {
			peeked := ring.queue.PeekBatch(ready)
			for i := int64(0); i < peeked; i++ {
				op := ready[i]
				ready[i] = nil
				op.ch <- Result{
					N:   0,
					Err: errors.New("uncompleted via closed"), // todo make err
				}
			}
		}
		// wg done
		ring.wg.Done()
	}(ctx, ring)
}

func (ring *Ring) listenCQ(ctx context.Context) {
	ring.wg.Add(1)
	go func(ctx context.Context, ring *Ring) {
		stopCh := ring.stopCh
		waitTimeout := syscall.NsecToTimespec(ring.waitTimeout.Nanoseconds()) // todo wait timeout
		cq := make([]*giouring.CompletionQueueEvent, ring.queue.capacity)
		stopped := false
		for {
			select {
			case <-ctx.Done():
				stopped = true
				break
			case <-stopCh:
				stopped = true
				break
			default:
				// wait
				if _, waitErr := ring.ring.WaitCQEs(1, &waitTimeout, nil); waitErr != nil {
					break
				}
				// peek
				completed := ring.ring.PeekBatchCQE(cq)
				if completed == 0 {
					break
				}
				// handle
				for i := uint32(0); i < completed; i++ {
					cqe := cq[i]
					cq[i] = nil
					if cqe.UserData == 0 {
						continue
					}
					//if cqe.Flags&giouring.CQEFNotif != 0 {
					//	// used by send_zc or sendmsg_ze, so continue
					//	continue
					//}
					// op
					cop := (*Operation)(unsafe.Pointer(uintptr(cqe.UserData)))

					//if cqe.Res == 0 {
					//	if cop.kind == acceptOp {
					//		fmt.Println("accept cqe zero:", cqe.Res, cqe.Flags, cqe.UserData)
					//	}
					//}
					if cop.done.CompareAndSwap(false, true) { // not done
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
					} else { // done
						// handle done but hijacked
						// 1. by timeout or ctx canceled, so should be hijacked
						// 2. by send_zc or sendmsg_zc, so should be hijacked
						if cop.hijacked.CompareAndSwap(true, false) {
							ring.ReleaseOperation(cop)
						}
					}
				}
				ring.ring.CQAdvance(completed)
				break
			}
			if stopped {
				break
			}
		}
		// wg done
		ring.wg.Done()
	}(ctx, ring)
}

func (ring *Ring) Stop() {
	if ring.stopCh != nil {
		close(ring.stopCh)
		ring.wg.Wait()
		ring.ring.QueueExit()
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
	case cancelOp:
		ptr := uint64(uintptr(unsafe.Pointer(op)))
		sqe.PrepareCancel64(ptr, 0)
		break
	default:
		sqe.PrepareNop()
		break
	}
	sqe.SetData(unsafe.Pointer(op))
	runtime.KeepAlive(sqe)
	return
}
