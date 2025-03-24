package queue

import (
	"github.com/brickingsoft/rio/pkg/iouring/aio/tagged"
	"sync"
	"sync/atomic"
	"unsafe"
)

func New[E any]() *Queue[E] {
	q := &Queue[E]{
		head: atomic.Uintptr{},
		tail: atomic.Uintptr{},
		len:  atomic.Int64{},
		ver:  atomic.Uintptr{},
		nds: sync.Pool{
			New: func() interface{} {
				return &node[E]{}
			},
		},
	}
	n := q.acquireNode()
	ptr := tagged.PointerPack[node[E]](unsafe.Pointer(n), 0)
	q.head.Store(uintptr(ptr))
	q.tail.Store(uintptr(ptr))
	return q
}

type node[E any] struct {
	entry *E
	next  atomic.Uintptr
}

type Queue[E any] struct {
	head atomic.Uintptr
	tail atomic.Uintptr
	len  atomic.Int64
	ver  atomic.Uintptr
	nds  sync.Pool
}

func (q *Queue[E]) acquireNode() *node[E] {
	n := q.nds.Get().(*node[E])
	return n
}

func (q *Queue[E]) releaseNode(n *node[E]) {
	q.nds.Put(n)
}

func (q *Queue[E]) Enqueue(entry *E) {
	n := q.acquireNode()
	n.entry = entry
	np := tagged.PointerPack[node[E]](unsafe.Pointer(n), q.ver.Add(1))
RETRY:
	var (
		tailPtr = q.tail.Load()
		tail    = tagged.Pointer[node[E]](tailPtr).Value()
		nextPtr = tail.next.Load()
	)

	if tailPtr == q.tail.Load() {
		if nextPtr == 0 {
			if tail.next.CompareAndSwap(nextPtr, uintptr(np)) {
				q.tail.CompareAndSwap(tailPtr, uintptr(np))
				q.len.Add(1)
				return
			}
		} else {
			q.tail.CompareAndSwap(tailPtr, nextPtr)
		}
	}
	goto RETRY
}

func (q *Queue[E]) Dequeue() *E {
RETRY:
	var (
		headPtr = q.head.Load()
		tailPtr = q.tail.Load()
		nextPtr = tagged.Pointer[node[E]](headPtr).Value().next.Load()
	)

	if headPtr == q.head.Load() {
		if headPtr == tailPtr {
			if nextPtr == 0 {
				return nil
			}
			q.tail.CompareAndSwap(tailPtr, nextPtr)
		} else {
			entry := tagged.Pointer[node[E]](nextPtr).Value().entry
			if q.head.CompareAndSwap(headPtr, nextPtr) {
				head := tagged.Pointer[node[E]](headPtr).Value()
				head.entry = nil
				head.next.Store(0)
				q.nds.Put(head)
				q.len.Add(-1)
				return entry
			}
		}
	}
	goto RETRY
}

func (q *Queue[E]) PeekBatch(entries []*E) (n uint32) {
	size := int64(len(entries))
	if size == 0 {
		return
	}
	entriesLen := q.Length()
	if entriesLen == 0 {
		return
	}
	if entriesLen < size {
		size = entriesLen
	}
	headPtr := q.head.Load()
	head := tagged.Pointer[node[E]](headPtr).Value()
	tailPtr := q.tail.Load()
	for i := int64(0); i < size; i++ {
		nextPtr := head.next.Load()
		if headPtr == tailPtr {
			if nextPtr == 0 {
				break
			}
			tailPtr = nextPtr
		} else {
			next := tagged.Pointer[node[E]](nextPtr).Value()
			if next == nil {
				break
			}
			entry := next.entry
			entries[i] = entry
			n++
			head = next
		}
	}
	return
}

func (q *Queue[E]) Advance(n uint32) {
	for i := uint32(0); i < n; i++ {
		if entry := q.Dequeue(); entry == nil {
			break
		}
	}
	return
}

func (q *Queue[E]) Length() int64 {
	return q.len.Load()
}
