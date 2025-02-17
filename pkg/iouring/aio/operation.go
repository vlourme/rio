package aio

import (
	"github.com/brickingsoft/rio/pkg/iouring"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

type Result struct {
	N     int
	Flags uint32
	Err   error
}

type pipeRequest struct {
	fdIn        int
	offIn       int64
	fdOut       int
	offOut      int64
	nbytes      uint32
	spliceFlags uint32
}

const (
	ReadyOperationStatus int64 = iota
	ProcessingOperationStatus
	CompletedOperationStatus
)

func NewOperation() *Operation {
	return &Operation{
		kind: iouring.OpLast,
		ch:   make(chan Result, 1),
	}
}

type Operation struct {
	kind     uint8
	borrowed bool
	status   atomic.Int64
	fd       int
	b        []byte
	msg      syscall.Msghdr
	pipe     pipeRequest
	ptr      unsafe.Pointer
	timeout  time.Duration
	ch       chan Result
}

func (op *Operation) WithTimeout(d time.Duration) *Operation {
	if d < 1 {
		return op
	}
	op.timeout = d
	return op
}

func (op *Operation) PrepareNop() (err error) {
	op.kind = iouring.OpNop
	return
}

func (op *Operation) PrepareAccept(fd int, addr *syscall.RawSockaddrAny, addrLen int) {
	op.kind = iouring.OpAccept
	op.fd = fd
	op.msg.Name = (*byte)(unsafe.Pointer(addr))
	op.msg.Namelen = uint32(addrLen)
	return
}

func (op *Operation) PrepareReceive(fd int, b []byte) {
	op.kind = iouring.OpRecv
	op.fd = fd
	op.b = b
	return
}

func (op *Operation) PrepareSend(fd int, b []byte) {
	op.kind = iouring.OpSend
	op.fd = fd
	op.b = b
	return
}

func (op *Operation) PrepareSendZC(fd int, b []byte) {
	op.kind = iouring.OpSendZC
	op.fd = fd
	op.b = b
	return
}

func (op *Operation) PrepareReceiveMsg(fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32) {
	op.kind = iouring.OpRecvmsg
	op.fd = fd
	op.setMsg(b, oob, addr, addrLen, flags)
	return
}

func (op *Operation) PrepareSendMsg(fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32) {
	op.kind = iouring.OpSendmsg
	op.fd = fd
	op.setMsg(b, oob, addr, addrLen, flags)
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

func (op *Operation) PrepareSendMsgZC(fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32) {
	op.kind = iouring.OpSendMsgZC
	op.fd = fd
	op.b = b
	op.setMsg(b, oob, addr, addrLen, flags)
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

func (op *Operation) PrepareSplice(fdIn int, offIn int64, fdOut int, offOut int64, nbytes uint32, flags uint32) {
	op.kind = iouring.OpSplice
	op.pipe.fdIn = fdIn
	op.pipe.offIn = offIn
	op.pipe.fdOut = fdOut
	op.pipe.offOut = offOut
	op.pipe.nbytes = nbytes
	op.pipe.spliceFlags = flags
}

func (op *Operation) PrepareTee(fdIn int, fdOut int, nbytes uint32, flags uint32) {
	op.kind = iouring.OpTee
	op.pipe.fdIn = fdIn
	op.pipe.fdOut = fdOut
	op.pipe.nbytes = nbytes
	op.pipe.spliceFlags = flags
}

func (op *Operation) PrepareCancel(target *Operation) {
	op.kind = iouring.OpAsyncCancel
	op.ptr = unsafe.Pointer(target)
}

func (op *Operation) reset() {
	// kind
	op.kind = iouring.OpLast
	// status
	op.status.Store(ReadyOperationStatus)
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
	// pipe
	op.pipe.fdIn = 0
	op.pipe.offIn = 0
	op.pipe.fdOut = 0
	op.pipe.offOut = 0
	op.pipe.nbytes = 0
	op.pipe.spliceFlags = 0
	// timeout
	op.timeout = 0
	// ptr
	op.ptr = nil
	return
}

func (op *Operation) setMsg(b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32) {
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

func newOperationQueue(n int) (queue *OperationQueue) {
	if n < 1 {
		n = iouring.DefaultEntries
	}
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
	value atomic.Pointer[Operation]
	next  atomic.Pointer[OperationQueueNode]
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
	runtime.KeepAlive(op)
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
