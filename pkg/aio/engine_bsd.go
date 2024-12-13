//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const (
	defaultChangesQueueSize = 16384
)

type KqueueSettings struct {
	ChangesQueueSize     int
	ChangesPeekBatchSize int
	EventsWaitBatchSize  int
	EventsWaitTimeout    time.Duration
}

func (engine *Engine) Start() {
	// settings
	settings := ResolveSettings[KqueueSettings](engine.settings)
	// cylinders
	for i := 0; i < len(engine.cylinders); i++ {
		cylinder, cylinderErr := newKqueueCylinder(settings.ChangesQueueSize, settings.ChangesPeekBatchSize, settings.EventsWaitBatchSize, settings.EventsWaitTimeout)
		if cylinderErr != nil {
			panic(fmt.Errorf("aio: engine start failed, %v", cylinderErr))
			return
		}
		engine.cylinders[i] = cylinder
	}
	for _, cylinder := range engine.cylinders {
		go func(engine *Engine, cylinder Cylinder) {
			if engine.cylindersLockOSThread {
				runtime.LockOSThread()
			}
			cylinder.Loop(engine.markCylinderLoop, engine.markCylinderStop)
			if engine.cylindersLockOSThread {
				runtime.UnlockOSThread()
			}
		}(engine, cylinder)
	}
}

func (engine *Engine) Stop() {
	runtime.SetFinalizer(engine, nil)

	for _, cylinder := range engine.cylinders {
		cylinder.Stop()
	}
	engine.wg.Wait()
}

func newKqueueCylinder(changesQueueSize int, changesPeekBatchSize int, eventsWaitBatchSize int, eventsWaitTimeout time.Duration) (cylinder *KqueueCylinder, err error) {
	if changesQueueSize < 1 {
		changesQueueSize = defaultChangesQueueSize
	}
	if changesPeekBatchSize < 1 {
		changesPeekBatchSize = changesQueueSize
	}
	if eventsWaitBatchSize < 1 {
		eventsWaitBatchSize = changesPeekBatchSize
	}
	if eventsWaitTimeout < 1 {
		eventsWaitTimeout = 1 * time.Millisecond
	}
	fd, fdErr := syscall.Kqueue()
	if fdErr != nil {
		err = os.NewSyscallError("kqueue", fdErr)
		return
	}

	pipe := make([]int, 2)

	if pipeErr := Pipe2(pipe); pipeErr != nil {
		err = pipeErr
		return
	}

	cylinder = &KqueueCylinder{
		fd:                   fd,
		pipe:                 pipe,
		changesPeekBatchSize: changesPeekBatchSize,
		eventsWaitBatchSize:  eventsWaitBatchSize,
		eventsWaitTimeout:    eventsWaitTimeout,
		sq:                   NewSubmissionQueue(changesQueueSize),
		completing:           atomic.Int64{},
		stopped:              atomic.Bool{},
		stopBytes:            []byte{'s'},
		wakeupBytes:          []byte{'w'},
	}

	wakeupEvent := cylinder.createPipeEvent([]byte{'w'})
	if _, wakeupErr := syscall.Kevent(fd, []syscall.Kevent_t{wakeupEvent}, nil, nil); wakeupErr != nil {
		_ = syscall.Close(fd)
		cylinder = nil
		err = os.NewSyscallError("kevent", wakeupErr)
		return
	}
	return
}

func nextKqueueCylinder() *KqueueCylinder {
	return nextCylinder().(*KqueueCylinder)
}

type KqueueCylinder struct {
	fd                   int
	pipe                 []int
	changesPeekBatchSize int
	eventsWaitBatchSize  int
	eventsWaitTimeout    time.Duration
	sq                   *SubmissionQueue
	completing           atomic.Int64
	stopped              atomic.Bool
	stopBytes            []byte
	wakeupBytes          []byte
}

func (cylinder *KqueueCylinder) Fd() int {
	return cylinder.fd
}

func (cylinder *KqueueCylinder) Actives() int64 {
	return cylinder.sq.Len() + cylinder.completing.Load()
}

func (cylinder *KqueueCylinder) Loop(beg func(), end func()) {
	beg()
	defer end()

	kqfd := cylinder.fd

	pipeReadFd := cylinder.pipe[0]
	pipeBuf := make([]byte, 8)
	stopBytes := cylinder.stopBytes
	wakeup := true
	wakeupBytes := cylinder.wakeupBytes

	changes := make([]syscall.Kevent_t, cylinder.changesPeekBatchSize)
	events := make([]syscall.Kevent_t, cylinder.eventsWaitBatchSize)
	timeout := cylinder.eventsWaitTimeout
	stopped := false
	for {
		if stopped {
			break
		}
		// deadline
		//deadline := time.Now().Add(timeout)
		//timespec := syscall.NsecToTimespec(deadline.UnixNano())
		// todo check timespec
		timespec := syscall.NsecToTimespec(int64(timeout))
		// peek
		peeked := cylinder.sq.PeekBatch(changes)
		if peeked == 0 && !wakeup {
			// wakeup
			wakeup = true
			changes[0] = cylinder.createPipeEvent(wakeupBytes)
			peeked = 1
		}

		n, err := syscall.Kevent(kqfd, changes[:peeked], events, &timespec)
		if err != nil {
			if errors.Is(err, syscall.EINTR) || errors.Is(err, syscall.ETIMEDOUT) {
				continue
			}
			cylinder.stopped.Store(true)
			stopped = true
			break
		}
		if n == 0 {
			continue
		}
		for i := 0; i < n; i++ {
			event := events[i]
			fd, data, eof, op := cylinder.deconstructEvent(event)
			if fd == pipeReadFd {
				rn, _ := syscall.Read(fd, pipeBuf)
				if rn > 0 {
					if bytes.Contains(pipeBuf[:rn], stopBytes) {
						cylinder.stopped.Store(true)
						stopped = true
						break
					}
					if bytes.Contains(pipeBuf[:rn], wakeupBytes) {
						wakeup = false
						continue
					}
				}
				continue
			}

			if op == nil {
				continue
			}

			cylinder.completing.Add(1)
			if completion := op.completion; completion != nil {
				// todo handle eof
				if eof {
					completion(int(data), op, ErrClosed)
				} else {
					completion(int(data), op, nil)
				}
				runtime.KeepAlive(op)
				op.callback = nil
				op.completion = nil
			}
			runtime.KeepAlive(op)
			cylinder.completing.Add(-1)
		}
	}
	if len(cylinder.pipe) == 2 {
		_ = syscall.Close(cylinder.pipe[0])
		_ = syscall.Close(cylinder.pipe[1])
	}
	if kqfd > 0 {
		_ = syscall.Close(kqfd)
	}
}

func (cylinder *KqueueCylinder) Stop() {
	if cylinder.stopped.Load() {
		return
	}
	event := cylinder.createPipeEvent(cylinder.stopBytes)
	for {
		if ok := cylinder.submit(&event); ok {
			break
		}
	}
}

func (cylinder *KqueueCylinder) submit(entry *syscall.Kevent_t) (ok bool) {
	if cylinder.stopped.Load() {
		return
	}
	ok = cylinder.sq.Enqueue(unsafe.Pointer(entry))
	runtime.KeepAlive(entry)
	return
}

func (cylinder *KqueueCylinder) prepareRead(fd int, op *Operator) (err error) {
	err = cylinder.prepareRW(fd, syscall.EVFILT_READ, syscall.EV_ADD|syscall.EV_ONESHOT|syscall.EV_CLEAR, op)
	return
}

func (cylinder *KqueueCylinder) prepareWrite(fd int, op *Operator) (err error) {
	err = cylinder.prepareRW(fd, syscall.EVFILT_WRITE, syscall.EV_ADD|syscall.EV_ONESHOT|syscall.EV_CLEAR, op)
	return
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

func (sq *SubmissionQueue) Enqueue(entry unsafe.Pointer) (ok bool) {
	for {
		if atomic.LoadInt64(&sq.entries) >= sq.capacity {
			return
		}
		tail := (*submissionQueueNode)(atomic.LoadPointer(&sq.tail))
		if tail.value != nil {
			continue
		}
		if atomic.CompareAndSwapPointer(&tail.value, tail.value, entry) {
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

func (sq *SubmissionQueue) Dequeue() (entry unsafe.Pointer) {
	for {
		head := (*submissionQueueNode)(atomic.LoadPointer(&sq.head))
		if head.value == nil {
			break
		}
		target := atomic.LoadPointer(&head.value)
		if atomic.CompareAndSwapPointer(&sq.head, sq.head, head.next) {
			atomic.AddInt64(&sq.entries, -1)
			entry = target
			break
		}
	}
	return
}

func (sq *SubmissionQueue) PeekBatch(entries []syscall.Kevent_t) (n int64) {
	size := int64(len(entries))
	if size == 0 {
		return
	}
	if num := atomic.LoadInt64(&sq.entries); num < size {
		size = num
	}
	for i := int64(0); i < size; i++ {
		ptr := sq.Dequeue()
		if ptr == nil {
			break
		}
		entries[i] = *((*syscall.Kevent_t)(ptr))
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
