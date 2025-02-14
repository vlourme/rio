package ring

import (
	"context"
	"github.com/brickingsoft/errors"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

type Result struct {
	N   int
	Err error
}

var (
	timers = sync.Pool{
		New: func() interface{} {
			return time.NewTimer(0)
		},
	}
	sendZCEnable    = false
	sendMsgZCEnable = false
)

func acquireTimer(d time.Duration) *time.Timer {
	timer := timers.Get().(*time.Timer)
	timer.Reset(d)
	return timer
}

func releaseTimer(t *time.Timer) {
	t.Stop()
	timers.Put(t)
}

type OperationKind int

const (
	nop OperationKind = iota
	acceptOp
	closeOp
	receiveOp
	sendOp
	sendZCOp
	receiveFromOp
	sendToOp
	receiveMsgOp
	sendMsgOp
	sendMsgZcOp
	spliceOp
	teeOp
	cancelOp
)

func (op *Operation) PrepareNop(fd int) (err error) {
	op.kind = nop
	op.fd = fd
	return
}

func (op *Operation) PrepareAccept(fd int) {
	op.kind = acceptOp
	op.fd = fd
	op.msg.Name = (*byte)(unsafe.Pointer(new(syscall.RawSockaddrAny)))
	op.msg.Namelen = uint32(syscall.SizeofSockaddrAny)
	return
}

func (op *Operation) PrepareReceive(fd int, b []byte) {
	op.kind = receiveOp
	op.fd = fd
	op.SetBytes(b)
	return
}

func (op *Operation) PrepareSend(fd int, b []byte) {
	if sendZCEnable {
		op.kind = sendZCOp
		op.hijacked.Store(true)
		op.hijackedBytes = b
	} else {
		op.kind = sendOp
	}
	op.fd = fd
	op.SetBytes(b)
	return
}

func (op *Operation) PrepareSendMsg(fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int) {
	if sendMsgZCEnable {
		op.kind = sendMsgZcOp
		op.hijacked.Store(true)
		op.hijackedBytes = b
		op.hijackedOOB = oob
		op.hijackedAddr = addr
		op.hijackedAddrLen = addrLen
	} else {
		op.kind = sendMsgOp
	}
	op.fd = fd
	op.msg.Name = (*byte)(unsafe.Pointer(addr))
	op.msg.Namelen = uint32(addrLen)
	op.SetBytes(b)
	op.SetControl(oob)
	/* todo handle of sotype != syscall.SOCK_DGRAM, 在外面处理
	if bLen == 0 && fd.SocketType() != syscall.SOCK_DGRAM {
				var dummy byte
				op.msg.Iov.Base = &dummy
				op.msg.Iov.Len = uint64(1)
				op.msg.Iovlen = 1
			}
	*/
	return
}

type Splice struct {
	fdIn        int
	offIn       int64
	fdOut       int
	offOut      int64
	nbytes      uint32
	spliceFlags uint32
}

type Operation struct {
	kind            OperationKind
	fd              int
	msg             syscall.Msghdr
	splice          Splice
	timeout         time.Duration
	done            atomic.Bool
	hijacked        atomic.Bool
	hijackedBytes   []byte
	hijackedOOB     []byte
	hijackedAddr    *syscall.RawSockaddrAny
	hijackedAddrLen int
	ch              chan Result
}

func (op *Operation) reset() bool {
	if op.hijacked.Load() {
		return false
	}
	// kind
	op.kind = nop
	// fd
	op.fd = 0
	// msg
	op.msg.Name = nil
	op.msg.Namelen = 0
	op.msg.Iov = nil
	op.msg.Iovlen = 0
	op.msg.Control = nil
	op.msg.Controllen = 0
	op.msg.Flags = 0
	// splice
	op.splice.fdIn = 0
	op.splice.offIn = 0
	op.splice.fdOut = 0
	op.splice.offOut = 0
	op.splice.nbytes = 0
	op.splice.spliceFlags = 0
	// timeout
	op.timeout = 0
	// done
	op.done.Store(false)
	// hijacked
	op.hijackedBytes = nil
	op.hijackedOOB = nil
	op.hijackedAddr = nil
	op.hijackedAddrLen = 0
	return true
}

func (op *Operation) SetTimeout(d time.Duration) {
	if d < 0 {
		d = 0
	}
	op.timeout = d
}

func (op *Operation) Discard() {
	op.hijacked.Store(false)
	op.done.Store(true)
}

// Await
// when err is ErrUncompleted, should call Ring.CancelOperation.
func (op *Operation) Await(ctx context.Context) (n int, err error) {
	ch := op.ch
	if timeout := op.timeout; timeout > 0 {
		timer := acquireTimer(op.timeout)
		timer.Reset(timeout)

		select {
		case r := <-ch:
			n, err = r.N, r.Err
			break
		case <-timer.C:
			if op.done.CompareAndSwap(false, true) {
				op.hijacked.Store(true)
				// result has not been sent
				err = errors.From(ErrUncompleted, errors.WithWrap(ErrTimeout))
			} else {
				// result has been sent, so continue to fetch result
				r := <-ch
				n, err = r.N, r.Err
			}
			break
		case <-ctx.Done():
			if op.done.CompareAndSwap(false, true) {
				op.hijacked.Store(true)
				err = errors.From(ErrUncompleted, errors.WithWrap(ctx.Err()))
			} else {
				r := <-ch
				n, err = r.N, r.Err
			}
			break
		}
		releaseTimer(timer)
	} else {
		select {
		case r := <-ch:
			n, err = r.N, r.Err
			break
		case <-ctx.Done():
			if op.done.CompareAndSwap(false, true) {
				op.hijacked.Store(true)
				err = errors.From(ErrUncompleted, errors.WithWrap(ctx.Err()))
			} else {
				r := <-ch
				n, err = r.N, r.Err
			}
			break
		}
	}
	return
}

func (op *Operation) SetBytes(b []byte) {
	bLen := len(b)
	if bLen == 0 {
		return
	}
	op.msg.Iov = &syscall.Iovec{
		Base: unsafe.SliceData(b),
		Len:  uint64(bLen),
	}
	op.msg.Iovlen = 1
}

func (op *Operation) AppendBytes(b []byte) {
	bLen := len(b)
	if bLen == 0 {
		return
	}
	bb := unsafe.Slice(op.msg.Iov, op.msg.Iovlen)
	bb = append(bb, syscall.Iovec{
		Base: unsafe.SliceData(b),
		Len:  uint64(bLen),
	})
	op.msg.Iov = unsafe.SliceData(bb)
	op.msg.Iovlen = uint64(len(bb))
}

func (op *Operation) SetAddr(name *syscall.RawSockaddrAny, nameLen int) {
	op.msg.Name = (*byte)(unsafe.Pointer(name))
	op.msg.Namelen = uint32(nameLen)
}

func (op *Operation) Addr() (addr *syscall.RawSockaddrAny, addrLen int) {
	addr = (*syscall.RawSockaddrAny)(unsafe.Pointer(op.msg.Name))
	addrLen = int(op.msg.Namelen)
	return
}

func (op *Operation) SetControl(b []byte) {
	bLen := len(b)
	if bLen == 0 {
		return
	}
	op.msg.Control = unsafe.SliceData(b)
	op.msg.Controllen = uint64(bLen)
}

func (op *Operation) Control() []byte {
	if op.msg.Control == nil || op.msg.Controllen == 0 {
		return nil
	}
	return unsafe.Slice(op.msg.Control, op.msg.Controllen)
}

func (op *Operation) ControlLen() int {
	return int(op.msg.Controllen)
}

func (op *Operation) SetFlags(flags int) {
	op.msg.Flags = int32(flags)
}

func (op *Operation) Flags() int {
	return int(op.msg.Flags)
}

func NewOperationQueue(n int) (queue *OperationQueue) {
	if n < 1 {
		n = 16384
	}
	n = RoundupPow2(n)
	queue = &OperationQueue{
		head:     nil,
		tail:     nil,
		entries:  0,
		capacity: int64(n),
	}
	hn := &OperationQueueNode{
		value: nil,
		next:  nil,
	}
	queue.head = unsafe.Pointer(hn)
	queue.tail = unsafe.Pointer(hn)

	for i := 1; i < n; i++ {
		next := &OperationQueueNode{}
		tail := (*OperationQueueNode)(atomic.LoadPointer(&queue.tail))
		tail.next = unsafe.Pointer(next)
		atomic.CompareAndSwapPointer(&queue.tail, queue.tail, unsafe.Pointer(next))
	}

	tail := (*OperationQueueNode)(atomic.LoadPointer(&queue.tail))
	tail.next = queue.head

	queue.tail = queue.head
	return
}

type OperationQueueNode struct {
	value unsafe.Pointer
	next  unsafe.Pointer
}

type OperationQueue struct {
	head     unsafe.Pointer
	tail     unsafe.Pointer
	entries  int64
	capacity int64
}

func (queue *OperationQueue) Enqueue(op *Operation) (ok bool) {
	ptr := unsafe.Pointer(op)
	for {
		if atomic.LoadInt64(&queue.entries) >= queue.capacity {
			break
		}
		tail := (*OperationQueueNode)(atomic.LoadPointer(&queue.tail))
		if tail.value != nil {
			continue
		}
		if atomic.CompareAndSwapPointer(&tail.value, tail.value, ptr) {
			for {
				if atomic.CompareAndSwapPointer(&queue.tail, queue.tail, tail.next) {
					atomic.AddInt64(&queue.entries, 1)
					ok = true
					return
				}
			}
		}
	}
	return
}

func (queue *OperationQueue) Dequeue() (op *Operation) {
	for {
		head := (*OperationQueueNode)(atomic.LoadPointer(&queue.head))
		if head.value == nil {
			break
		}
		target := atomic.LoadPointer(&head.value)
		if atomic.CompareAndSwapPointer(&queue.head, queue.head, head.next) {
			atomic.StorePointer(&head.value, nil)
			atomic.AddInt64(&queue.entries, -1)
			op = (*Operation)(target)
			break
		}
	}
	return
}

func (queue *OperationQueue) PeekBatch(operations []*Operation) (n int64) {
	size := int64(len(operations))
	if size == 0 {
		return
	}
	if num := atomic.LoadInt64(&queue.entries); num < size {
		size = num
	}
	node := (*OperationQueueNode)(atomic.LoadPointer(&queue.head))
	for i := int64(0); i < size; i++ {
		if node.value == nil {
			break
		}
		target := atomic.LoadPointer(&node.value)
		node = (*OperationQueueNode)(atomic.LoadPointer(&node.next))
		op := (*Operation)(target)
		operations[i] = op
		n++
	}
	return
}

func (queue *OperationQueue) Advance(n int64) {
	for i := int64(0); i < n; i++ {
		head := (*OperationQueueNode)(atomic.LoadPointer(&queue.head))
		atomic.StorePointer(&head.value, nil)
		if atomic.CompareAndSwapPointer(&queue.head, queue.head, head.next) {
			atomic.AddInt64(&queue.entries, -1)
		}
	}
	return
}

func (queue *OperationQueue) Len() int64 {
	return atomic.LoadInt64(&queue.entries)
}

func (queue *OperationQueue) Cap() int64 {
	return queue.capacity
}
