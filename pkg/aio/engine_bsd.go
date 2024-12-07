//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

import (
	"runtime"
	"sync/atomic"
	"unsafe"
)

func (engine *Engine) Start() {

}

func (engine *Engine) Stop() {

}

type KqueueCylinder struct {
	fd         int
	sq         *SubmissionQueue
	completing atomic.Int64
	stopped    atomic.Bool
}

func (cylinder *KqueueCylinder) Fd() int {
	return cylinder.fd
}

func (cylinder *KqueueCylinder) Loop(beg func(), end func()) {
	// todo
	// 与ring类似，prepare rw 到一个无锁queue。
	// loop 里 submit queue。注意 submit 是一次性的。
	// active 也是 queue 的 ready
	//TODO implement me
	panic("implement me")
}

func (cylinder *KqueueCylinder) Stop() {
	if cylinder.stopped.Load() {
		return
	}
	cylinder.stopped.Store(true)
	// todo submit a no-op entry
	//TODO implement me
	panic("implement me")
}

func (cylinder *KqueueCylinder) Actives() int64 {
	return cylinder.sq.Len() + cylinder.completing.Load()
}

func (cylinder *KqueueCylinder) submit(entry SubmissionQueueEntry) (ok bool) {
	if cylinder.stopped.Load() {
		return
	}
	ok = cylinder.sq.Enqueue(&entry)
	runtime.KeepAlive(entry)
	return
}

type SubmissionQueueEntry struct {
	Op     *Operator
	Flags  uint16
	Filter int16
}

type submissionQueueNode struct {
	value unsafe.Pointer
	next  unsafe.Pointer
}

func NewSubmissionQueue(n int) (sq *SubmissionQueue) {
	if n < 1 {
		n = 16384
	}
	n = RoundupPow2(n)
	sq = &SubmissionQueue{
		head:     nil,
		tail:     nil,
		entries:  0,
		capacity: int64(n),
	}
	hn := &submissionQueueNode{
		value: nil,
		next:  nil,
	}
	sq.head = unsafe.Pointer(hn)
	sq.tail = unsafe.Pointer(hn)

	for i := 1; i < n; i++ {
		next := &submissionQueueNode{}
		tail := (*submissionQueueNode)(atomic.LoadPointer(&sq.tail))
		tail.next = unsafe.Pointer(next)
		atomic.CompareAndSwapPointer(&sq.tail, sq.tail, unsafe.Pointer(next))
	}

	tail := (*submissionQueueNode)(atomic.LoadPointer(&sq.tail))
	tail.next = sq.head

	sq.tail = sq.head
	return
}

type SubmissionQueue struct {
	head     unsafe.Pointer
	tail     unsafe.Pointer
	entries  int64
	capacity int64
}

func (sq *SubmissionQueue) Enqueue(entry *SubmissionQueueEntry) (ok bool) {
	if entry == nil {
		return
	}
	for {
		if atomic.LoadInt64(&sq.entries) >= sq.capacity {
			return
		}
		tail := (*submissionQueueNode)(atomic.LoadPointer(&sq.tail))
		if tail.value != nil {
			continue
		}
		if atomic.CompareAndSwapPointer(&tail.value, tail.value, unsafe.Pointer(entry)) {
			for {
				if atomic.CompareAndSwapPointer(&sq.tail, sq.tail, tail.next) {
					atomic.AddInt64(&sq.entries, 1)
					ok = true
					return
				}
			}
		}
	}
}

func (sq *SubmissionQueue) Dequeue() (entry *SubmissionQueueEntry) {
	for {
		head := (*submissionQueueNode)(atomic.LoadPointer(&sq.head))
		if head.value == nil {
			break
		}
		target := (*SubmissionQueueEntry)(atomic.LoadPointer(&head.value))
		if atomic.CompareAndSwapPointer(&sq.head, sq.head, head.next) {
			atomic.AddInt64(&sq.entries, -1)
			entry = target
			break
		}
	}
	return
}

func (sq *SubmissionQueue) PeekBatch(entries []*SubmissionQueueEntry) (n int64) {
	size := int64(len(entries))
	if size == 0 {
		return
	}
	if num := atomic.LoadInt64(&sq.entries); num < size {
		size = num
	}
	for i := int64(0); i < size; i++ {
		entry := sq.Dequeue()
		if entry == nil {
			break
		}
		entries[i] = entry
		n++
	}
	return
}

func (sq *SubmissionQueue) Len() int64 {
	return atomic.LoadInt64(&sq.entries)
}

func (sq *SubmissionQueue) Cap() int64 {
	return sq.capacity
}
