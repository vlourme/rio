package aio

import (
	"sync/atomic"
)

func NewQueue[E any]() *Queue[E] {
	node := &queueNode[E]{}
	q := &Queue[E]{}
	q.head.Store(node)
	q.tail.Store(node)
	return q
}

type queueNode[E any] struct {
	entry *E
	next  atomic.Pointer[queueNode[E]]
}

type Queue[E any] struct {
	head atomic.Pointer[queueNode[E]]
	tail atomic.Pointer[queueNode[E]]
	len  atomic.Int64
}

func (q *Queue[E]) Enqueue(entry *E) {
	node := &queueNode[E]{entry: entry}
RETRY:
	var (
		tail = q.tail.Load()
		next = tail.next.Load()
	)

	if tail == q.tail.Load() {
		if next == nil {
			if tail.next.CompareAndSwap(next, node) {
				q.tail.CompareAndSwap(tail, node)
				q.len.Add(1)
				return
			}
		} else {
			q.tail.CompareAndSwap(tail, next)
		}
	}
	goto RETRY
}

func (q *Queue[E]) Dequeue() *E {
RETRY:
	var (
		head = q.head.Load()
		tail = q.tail.Load()
		next = head.next.Load()
	)

	if head == q.head.Load() {
		if head == tail {
			if next == nil {
				return nil
			}
			q.tail.CompareAndSwap(tail, next)
		} else {
			entry := next.entry
			if q.head.CompareAndSwap(head, next) {
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
	if entriesLen := q.Length(); entriesLen < size {
		size = entriesLen
	}
	head := q.head.Load()
	tail := q.tail.Load()
	for i := int64(0); i < size; i++ {
		next := head.next.Load()
		if head == tail {
			if next == nil {
				break
			}
			tail = next
		} else {
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
