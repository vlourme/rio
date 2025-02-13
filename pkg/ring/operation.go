package ring

import (
	"github.com/pawelgaczynski/giouring"
	"sync/atomic"
	"syscall"
	"unsafe"
)

type Result struct {
	N   int
	Err error
}

type OperationKind int

const (
	Nop OperationKind = iota
	AcceptOp
	CloseOp
	ReceiveOp
	SendOp
	ReceiveFrom
	SendTo
	ReceiveMsg
	SendMsg
	SpliceOp
	TeeOp
)

func PrepareNop(ch chan Result) *Operation {
	return &Operation{
		kind: Nop,
		fd:   0,
		msg:  syscall.Msghdr{},
		ch:   ch,
	}
}

func PrepareAccept(fd int, ch chan Result) *Operation {
	addr := new(syscall.RawSockaddrAny)
	addrLen := syscall.SizeofSockaddrAny
	return &Operation{
		kind: AcceptOp,
		fd:   fd,
		msg: syscall.Msghdr{
			Name:    (*byte)(unsafe.Pointer(addr)),
			Namelen: uint32(addrLen),
		},
		ch: ch,
	}
}

type Operation struct {
	kind OperationKind
	fd   int
	msg  syscall.Msghdr
	ch   chan Result
}

func (op *Operation) Await() (int, error) {
	// todo add timeout
	r := <-op.ch
	return r.N, r.Err
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

func (op *Operation) prepare(sqe *giouring.SubmissionQueueEntry) {
	switch op.kind {
	case Nop:
		sqe.PrepareNop()
		sqe.SetData(unsafe.Pointer(&op.ch))
		break
	case AcceptOp:
		addrPtr := uintptr(unsafe.Pointer(op.msg.Name))
		addrLenPtr := uint64(uintptr(unsafe.Pointer(&op.msg.Namelen)))
		sqe.PrepareAccept(op.fd, addrPtr, addrLenPtr, 0)
		sqe.SetData(unsafe.Pointer(&op.ch))
		break
	default:
		sqe.PrepareNop()
		sqe.SetData(unsafe.Pointer(&op.ch))
		break
	}
	return
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
