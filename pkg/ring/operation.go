package ring

import (
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var (
	sendZCEnable    = false
	sendMsgZCEnable = false
)

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

func (op *Operation) PrepareNop() (err error) {
	op.kind = nop
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
	op.b = b
	return
}

func (op *Operation) PrepareSend(fd int, b []byte) {
	if sendZCEnable {
		op.kind = sendZCOp
	} else {
		op.kind = sendOp
	}
	op.fd = fd
	op.b = b
	return
}

func (op *Operation) PrepareSendMsg(fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int) {
	if sendMsgZCEnable {
		op.kind = sendMsgZcOp
	} else {
		op.kind = sendMsgOp
	}
	op.fd = fd
	op.SetMsg(b, oob, addr, addrLen, 0)
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
	kind    OperationKind
	fd      int
	b       []byte
	msg     syscall.Msghdr
	splice  Splice
	timeout time.Duration
	done    atomic.Bool
	ch      chan Result
}

func (op *Operation) Fd() int {
	return op.fd
}

func (op *Operation) SetFd(fd int) {
	op.fd = fd
}

func (op *Operation) Kind() OperationKind {
	return op.kind
}

func (op *Operation) reset() {
	// kind
	op.kind = nop
	// fd
	op.fd = 0
	// b
	op.b = nil
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
	return
}

func (op *Operation) SetTimeout(d time.Duration) {
	if d < 0 {
		d = 0
	}
	op.timeout = d
}

func (op *Operation) SetMsg(b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32) {
	bLen := len(b)
	oobLen := len(oob)
	if bLen > 0 {
		op.msg.Iov.Base = &b[0]
		op.msg.Iov.SetLen(bLen)
		op.msg.Iovlen = 1
	}
	if oobLen > 0 {
		op.msg.Control = &oob[0]
		op.msg.SetControllen(oobLen)
	}
	if addr != nil {
		op.msg.Name = (*byte)(unsafe.Pointer(addr))
		op.msg.Namelen = uint32(addrLen)
	}
	op.msg.Flags = flags
	return
}

func (op *Operation) Addr() (addr *syscall.RawSockaddrAny, addrLen int) {
	addr = (*syscall.RawSockaddrAny)(unsafe.Pointer(op.msg.Name))
	addrLen = int(op.msg.Namelen)
	return
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

func (op *Operation) Flags() int {
	return int(op.msg.Flags)
}

func NewOperationQueue(n int) (queue *OperationQueue) {
	if n < 1 {
		n = 16384
	}
	n = RoundupPow2(n)
	queue = &OperationQueue{
		head:     atomic.Pointer[OperationQueueNode]{},
		tail:     atomic.Pointer[OperationQueueNode]{},
		entries:  atomic.Int64{},
		capacity: int64(n),
	}
	head := &OperationQueueNode{
		value: atomic.Pointer[Operation]{},
		next:  atomic.Pointer[OperationQueueNode]{},
	}
	queue.head.Store(head)
	queue.tail.Store(head)

	for i := 1; i < n; i++ {
		next := &OperationQueueNode{}
		tail := queue.tail.Load()
		tail.next.Store(next)
		queue.tail.CompareAndSwap(tail, next)
	}

	tail := queue.tail.Load()
	tail.next.Store(head)

	queue.tail.Store(head)
	return
}

type OperationQueueNode struct {
	//value unsafe.Pointer
	value atomic.Pointer[Operation]
	//next  unsafe.Pointer
	next atomic.Pointer[OperationQueueNode]
}

type OperationQueue struct {
	head     atomic.Pointer[OperationQueueNode]
	tail     atomic.Pointer[OperationQueueNode]
	entries  atomic.Int64
	capacity int64
}

func (queue *OperationQueue) Enqueue(op *Operation) (ok bool) {
	for {
		if queue.entries.Load() >= queue.capacity {
			break
		}
		tail := queue.tail.Load()
		if tail.value.CompareAndSwap(nil, op) {
			next := tail.next.Load()
			queue.tail.Store(next)
			queue.entries.Add(1)
			ok = true
			break
		}
	}
	return
}

func (queue *OperationQueue) Dequeue() (op *Operation) {
	for {
		head := queue.head.Load()
		target := head.value.Load()
		if target == nil {
			break
		}
		next := head.next.Load()
		if queue.head.CompareAndSwap(head, next) {
			head.value.Store(nil)
			queue.entries.Add(-1)
			op = target
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
	if entriesLen := queue.entries.Load(); entriesLen < size {
		size = entriesLen
	}
	node := queue.head.Load()
	for i := int64(0); i < size; i++ {
		op := node.value.Load()
		if op == nil {
			break
		}
		node = node.next.Load()
		operations[i] = op
		n++
	}
	return
}

func (queue *OperationQueue) Advance(n int64) {
	node := queue.head.Load()
	for i := int64(0); i < n; i++ {
		node.value.Store(nil)
		node = node.next.Load()
		queue.entries.Add(-1)
	}
	queue.head.Store(node)
	return
}

func (queue *OperationQueue) Len() int64 {
	return queue.entries.Load()
}

func (queue *OperationQueue) Cap() int64 {
	return queue.capacity
}
