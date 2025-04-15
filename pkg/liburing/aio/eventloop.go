//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"math"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

type Promise interface {
	Complete(n int, flags uint32, err error)
}

type PromiseAdaptor interface {
	Handle(n int, flags uint32, err error) (bool, int, uint32, unsafe.Pointer, error)
}

type CompletionEvent struct {
	N          int
	Flags      uint32
	Err        error
	Attachment unsafe.Pointer
}

type Future interface {
	Await() (n int, flags uint32, attachment unsafe.Pointer, err error)
	AwaitDeadline(deadline time.Time) (n int, flags uint32, attachment unsafe.Pointer, err error)
	AwaitBatch(hungry bool, deadline time.Time) (events []CompletionEvent)
}

func newEventLoop(id int, group *EventLoopGroup, options Options) (v <-chan *EventLoop) {
	ch := make(chan *EventLoop)
	go func(id int, group *EventLoopGroup, options Options, ch chan<- *EventLoop) {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		opts := make([]liburing.Option, 0, 1)
		opts = append(opts, liburing.WithEntries(options.Entries))
		opts = append(opts, liburing.WithFlags(options.Flags))
		if options.Flags&liburing.IORING_SETUP_SQPOLL != 0 {
			opts = append(opts, liburing.WithSQThreadIdle(options.SQThreadIdle))
			if options.Flags&liburing.IORING_SETUP_SQ_AFF != 0 {
				opts = append(opts, liburing.WithSQThreadCPU(uint32(id)))
			}
		}

		ring, ringErr := liburing.New(opts...)
		if ringErr != nil {
			event := &EventLoop{
				id:  id,
				err: ringErr,
			}
			ch <- event
			return
		}

		// buffer and rings
		brs, brsErr := newBufferAndRings(options.BufferAndRingConfig)
		if brsErr != nil {
			_ = ring.Close()
			event := &EventLoop{
				id:  id,
				err: brsErr,
			}
			ch <- event
			return
		}
		// register files
		if _, regErr := ring.RegisterFilesSparse(uint32(math.MaxUint16)); regErr != nil {
			_ = ring.Close()
			event := &EventLoop{
				id:  id,
				err: regErr,
			}
			ch <- event
			return
		}
		// wait idle timeout
		waitIdleTimeout := options.WaitCQEIdleTimeout
		if waitIdleTimeout < 1 {
			waitIdleTimeout = 15 * time.Second
		}
		// wait transmission

		event := &EventLoop{
			ring:            ring,
			bufferAndRings:  brs,
			group:           group,
			wg:              new(sync.WaitGroup),
			id:              id,
			key:             0,
			running:         atomic.Bool{},
			idle:            atomic.Bool{},
			waitIdleTimeout: waitIdleTimeout,
			waitTimeCurve:   options.WaitCQETimeCurve,
			ready:           make(chan *Operation, ring.SQEntries()),
			err:             nil,
		}
		event.key = uint64(uintptr(unsafe.Pointer(event)))
		// return event loop
		ch <- event
		close(ch)
		// start buffer and rings loop
		brs.Start(event)
		// process
		event.process()
	}(id, group, options, ch)
	v = ch
	return
}

type EventLoop struct {
	ring            *liburing.Ring
	bufferAndRings  *BufferAndRings
	group           *EventLoopGroup
	wg              *sync.WaitGroup
	id              int
	key             uint64
	running         atomic.Bool
	idle            atomic.Bool
	waitIdleTimeout time.Duration
	waitTimeCurve   Curve
	submitter       func(op *Operation) (future Future)
	ready           chan *Operation
	orphan          *Operation
	err             error
}

func (eventLoop *EventLoop) Id() int {
	return eventLoop.id
}

func (eventLoop *EventLoop) Key() uint64 {
	return eventLoop.key
}

func (eventLoop *EventLoop) Fd() int {
	return eventLoop.ring.Fd()
}

func (eventLoop *EventLoop) Valid() error {
	return eventLoop.err
}

func (eventLoop *EventLoop) Group() *EventLoopGroup {
	return eventLoop.group
}

func (eventLoop *EventLoop) Submit(op *Operation) (future Future) {
	future = eventLoop.submitter(op)
	return
}

func (eventLoop *EventLoop) SubmitAndWait(op *Operation) (n int, flags uint32, err error) {
	n, flags, _, err = eventLoop.Submit(op).Await()
	return
}

func (eventLoop *EventLoop) Cancel(target *Operation) (err error) {
	op := AcquireOperation()
	op.PrepareCancel(target)
	_, _, err = eventLoop.SubmitAndWait(op)
	ReleaseOperation(op)
	return
}

func (eventLoop *EventLoop) AcquireBufferAndRing() (br *BufferAndRing, err error) {
	op := AcquireOperation()
	op.kind = op_kind_noexec
	op.cmd = op_cmd_acquire_br
	_, _, err = eventLoop.SubmitAndWait(op)
	if err == nil {
		br = (*BufferAndRing)(op.addr)
	}
	ReleaseOperation(op)
	return
}

func (eventLoop *EventLoop) ReleaseBufferAndRing(br *BufferAndRing) {
	eventLoop.bufferAndRings.Release(br)
}

func (eventLoop *EventLoop) Close() (err error) {
	// stop buffer and rings
	eventLoop.bufferAndRings.Stop()
	// submit close op
	op := &Operation{}
	op.PrepareCloseRing(eventLoop.key)
	eventLoop.Submit(op)
	// wait
	eventLoop.wg.Wait()
	// close ready
	close(eventLoop.ready)
	// get err
	err = eventLoop.err
	return
}

func (eventLoop *EventLoop) process() {
	if eventLoop.ring.Flags()&liburing.IORING_SETUP_SINGLE_ISSUER == 0 {
		eventLoop.submitter = eventLoop.submit2
		eventLoop.process2()
	} else {
		_ = sys.AffCPU(eventLoop.id)
		eventLoop.submitter = eventLoop.submit1
		eventLoop.process1()
	}
}

func setupOpCh(op *Operation) *Channel {
	channel := acquireChannel(op.kind == op_kind_multishot)
	if op.timeout != nil {
		op.timeout.channel = acquireChannel(false)
		channel.timeout = op.timeout.channel
	}
	op.channel = channel
	return channel
}

func (eventLoop *EventLoop) submit1(op *Operation) (future Future) {
	channel := setupOpCh(op)
	future = channel
	if eventLoop.running.Load() {
		eventLoop.ready <- op
		if eventLoop.idle.CompareAndSwap(true, false) {
			if err := eventLoop.group.wakeup.Wakeup(eventLoop.ring.Fd()); err != nil {
				channel.Complete(0, 0, ErrCanceled)
				return
			}
		}
		return
	}
	channel.Complete(0, 0, ErrCanceled)
	return
}

func (eventLoop *EventLoop) process1() {
	eventLoop.wg.Add(1)
	defer eventLoop.wg.Done()

	eventLoop.running.Store(true)

	var (
		stopped      bool
		ready        = eventLoop.ready
		readyN       uint32
		completed    uint32
		waitNr       uint32
		waitTimeout  *syscall.Timespec
		waitIdleTime = syscall.NsecToTimespec(eventLoop.waitIdleTimeout.Nanoseconds())
		ring         = eventLoop.ring
		transmission = NewCurveTransmission(eventLoop.waitTimeCurve)
		cqes         = make([]*liburing.CompletionQueueEvent, eventLoop.ring.CQEntries())
	)
	_, _ = ring.RegisterRingFd()

	for {
		// prepare
		if eventLoop.orphan != nil {
			if !eventLoop.prepareSQE(eventLoop.orphan) {
				goto SUBMIT
			}
			eventLoop.orphan = nil
		}
		if readyN = uint32(len(ready)); readyN > 0 {
			for i := uint32(0); i < readyN; i++ {
				op := <-ready
				if !eventLoop.prepareSQE(op) {
					eventLoop.orphan = op
					break
				}
			}
		}
		// submit
	SUBMIT:
		if readyN == 0 && completed == 0 { // idle
			if eventLoop.idle.CompareAndSwap(false, true) {
				waitNr, waitTimeout = 1, &waitIdleTime
			} else {
				waitNr, waitTimeout = transmission.Match(1)
			}
		} else {
			waitNr, waitTimeout = transmission.Match(readyN + completed)
		}
		_, _ = ring.SubmitAndWaitTimeout(waitNr, waitTimeout, nil)

		// reset idle
		eventLoop.idle.CompareAndSwap(true, false)

		// complete
		if completed, stopped = eventLoop.completeCQE(&cqes); stopped {
			break
		}
	}

	// unregister buffer and rings
	_ = eventLoop.bufferAndRings.Unregister()
	/* unregister files
	when kernel is less than 6.13, unregister files maybe blocked.
	so use a timer and done to fix it.
	*/
	unregisterFilesDone := make(chan struct{})
	unregisterFilesTimer := time.NewTimer(10 * time.Millisecond)
	go func(ring *liburing.Ring, done chan struct{}) {
		_, _ = ring.UnregisterFiles()
		close(unregisterFilesDone)
	}(ring, unregisterFilesDone)
	select {
	case <-unregisterFilesTimer.C:
		break
	case <-unregisterFilesDone:
		break
	}
	unregisterFilesTimer.Stop()
	// close ring
	eventLoop.err = eventLoop.ring.Close()
	return
}

func (eventLoop *EventLoop) submit2(op *Operation) (future Future) {
	channel := setupOpCh(op)
	future = channel
	if eventLoop.running.Load() {
		eventLoop.ready <- op
		return
	}
	channel.Complete(0, 0, ErrCanceled)
	return
}

func (eventLoop *EventLoop) process2() {
	eventLoop.wg.Add(1)
	defer eventLoop.wg.Done()

	eventLoop.running.Store(true)
	done := make(chan struct{})

	go eventLoop.asyncWaiting(done)

	var (
		ring    = eventLoop.ring
		ready   = eventLoop.ready
		readyN  uint32
		orphan  *Operation
		stopped bool
	)

	for {
		// prepare
		if orphan != nil {
			if !eventLoop.prepareSQE(orphan) {
				_, _ = ring.Submit()
				continue
			}
			orphan = nil
		}
		// ready
		if readyN = uint32(len(ready)); readyN > 0 {
			for i := uint32(0); i < readyN; i++ {
				op := <-ready
				if !eventLoop.prepareSQE(op) {
					orphan = op
					break
				}
			}
			_, _ = ring.Submit()
			continue
		}
		// wait
		select {
		case op := <-ready:
			if !eventLoop.prepareSQE(op) {
				orphan = op
				break
			}
			_, _ = ring.Submit()
			break
		case <-done:
			stopped = true
			break
		}
		if stopped {
			break
		}
	}

}

func (eventLoop *EventLoop) asyncWaiting(done chan<- struct{}) {
	var (
		ring         = eventLoop.ring
		stopped      bool
		batchCN      uint32
		completed    uint32
		waitNr       uint32
		waitTimeout  *syscall.Timespec
		waitIdleTime = syscall.NsecToTimespec(eventLoop.waitIdleTimeout.Nanoseconds())
		transmission = NewCurveTransmission(eventLoop.waitTimeCurve)
		cqes         = make([]*liburing.CompletionQueueEvent, eventLoop.ring.CQEntries())
	)

	for {
		waitNr, waitTimeout = transmission.Match(completed)
		if completed == 0 {
			_, _ = ring.WaitCQETimeout(&waitIdleTime)
		} else {
			_, _ = ring.WaitCQEs(waitNr, waitTimeout, nil)
		}
		completed = 0
	PEEK:
		if peeked := ring.PeekBatchCQE(cqes); peeked > 0 {
			// complete
			if batchCN, stopped = eventLoop.completeCQE(&cqes); stopped {
				break
			}
			completed = batchCN
			if completed > 0 {
				goto PEEK
			}
		}

	}
	close(done)
}

func (eventLoop *EventLoop) prepareSQE(op *Operation) bool {
	// handle noexec op
	if op.kind == op_kind_noexec {
		switch op.cmd {
		case op_cmd_acquire_br:
			br, brErr := eventLoop.bufferAndRings.Acquire()
			if brErr != nil {
				op.complete(0, 0, brErr)
				break
			}
			op.addr = unsafe.Pointer(br)
			op.complete(1, 0, nil)
			break
		case op_cmd_close_br:
			br := (*BufferAndRing)(op.addr)
			if err := br.free(eventLoop.ring); err != nil {
				op.complete(0, 0, err)
			} else {
				op.complete(1, 0, nil)
			}
			break
		default:
			op.complete(0, 0, errors.New("invalid command"))
			break
		}
		return true
	}
	// get sqe
	sqe := eventLoop.ring.GetSQE()
	if sqe == nil {
		return false
	}
	// prepare timeout timeout
	if op.timeout != nil {
		timeoutSQE := eventLoop.ring.GetSQE()
		if timeoutSQE == nil { // no sqe left, prepare nop and set op to head or ready
			sqe.PrepareNop()
			return false
		}
		// packing
		if err := op.packingSQE(sqe); err != nil {
			op.complete(0, 0, err)
			sqe.PrepareNop()
			return true
		}
		// packing timeout
		if err := op.timeout.packingSQE(timeoutSQE); err != nil {
			op.complete(0, 0, err)
			sqe.PrepareNop()
			return true
		}
		return true
	}
	// packing
	if err := op.packingSQE(sqe); err != nil {
		op.complete(0, 0, err)
		sqe.PrepareNop()
		return true
	}
	return true
}

func (eventLoop *EventLoop) completeCQE(cqesp *[]*liburing.CompletionQueueEvent) (completed uint32, stopped bool) {
	ring := eventLoop.ring
	cqes := *cqesp
	if peeked := ring.PeekBatchCQE(cqes); peeked > 0 {
		for i := uint32(0); i < peeked; i++ {
			cqe := cqes[i]
			cqes[i] = nil

			if cqe.UserData == 0 { // no userdata means no op
				ring.CQAdvance(1)
				continue
			}

			if cqe.UserData == eventLoop.key { // userdata is key means closed
				ring.CQAdvance(1)
				eventLoop.running.Store(true)
				stopped = true
				continue
			}

			// get op from cqe
			copPtr := cqe.GetData()
			cop := (*Operation)(copPtr)
			var (
				opN     = int(cqe.Res)
				opFlags = cqe.Flags
				opErr   error
			)
			if opN < 0 {
				if -opN == int(syscall.ECANCELED) {
					opErr = ErrCanceled
				} else {
					opErr = os.NewSyscallError(cop.Name(), syscall.Errno(-opN))
				}
				opN = 0
			}

			cop.complete(opN, opFlags, opErr)

			ring.CQAdvance(1)

		}
		completed += peeked
	}
	return
}
