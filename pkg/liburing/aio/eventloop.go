//go:build linux

package aio

import (
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
		// poll
		poll := ring.Flags()&liburing.IORING_SETUP_SQPOLL != 0

		// register personality
		personality, _ := ring.RegisterPersonality()

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

		eventLoop := &EventLoop{
			ring:            ring,
			bufferAndRings:  brs,
			group:           group,
			wg:              new(sync.WaitGroup),
			id:              id,
			key:             uint64(uintptr(unsafe.Pointer(ring))),
			personality:     uint16(personality),
			running:         atomic.Bool{},
			idle:            atomic.Bool{},
			poll:            poll,
			waitIdleTimeout: waitIdleTimeout,
			waitTimeCurve:   options.WaitCQETimeCurve,
			ready:           make(chan *Operation, ring.SQEntries()),
			err:             nil,
		}
		eventLoop.running.Store(true)
		// return event loop
		ch <- eventLoop
		close(ch)
		// start buffer and rings loop
		brs.Start(eventLoop)
		// process
		eventLoop.process()
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
	personality     uint16
	running         atomic.Bool
	idle            atomic.Bool
	poll            bool
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
	register := acquireBufferAndRingRegister(eventLoop)
	var ptr unsafe.Pointer
	op := AcquireOperation()
	op.PrepareCreateBufferAndRing(register)
	_, _, ptr, err = eventLoop.submitter(op).Await()
	if err == nil {
		br = (*BufferAndRing)(ptr)
	}
	ReleaseOperation(op)
	releaseBufferAndRingRegister(register)
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
	eventLoop.wg.Add(1)
	defer eventLoop.wg.Done()

	if eventLoop.poll {
		eventLoop.submitter = eventLoop.submit2
		eventLoop.process2()
	} else {
		_ = sys.AffCPU(eventLoop.id)
		eventLoop.submitter = eventLoop.submit1
		eventLoop.process1()
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
	}(eventLoop.ring, unregisterFilesDone)
	select {
	case <-unregisterFilesTimer.C:
		break
	case <-unregisterFilesDone:
		break
	}
	unregisterFilesTimer.Stop()
	// close ring
	eventLoop.err = eventLoop.ring.Close()
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
	op.personality = eventLoop.personality
	if eventLoop.running.Load() {
		eventLoop.ready <- op
		if eventLoop.idle.CompareAndSwap(true, false) {
			if err := eventLoop.group.wakeup.Wakeup(eventLoop.ring.Fd(), 0); err != nil {
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
	waitNr, waitTimeout = transmission.Match(1)
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
		// submit and wait
	SUBMIT:
		if readyN == 0 && completed == 0 { // idle
			if eventLoop.idle.CompareAndSwap(false, true) {
				waitNr, waitTimeout = 1, &waitIdleTime
			} else {
				waitNr, waitTimeout = transmission.Match(1)
			}
		} else {
			// adjust wait timeout
			waitNr, waitTimeout = transmission.Match(readyN + completed)
		}
		_, _ = ring.SubmitAndWaitTimeout(waitNr, waitTimeout, nil)
		// reset idle
		eventLoop.idle.CompareAndSwap(true, false)
		// handle complete
		if completed, stopped = eventLoop.completeCQE(&cqes); stopped {
			break
		}
	}
	return
}

func (eventLoop *EventLoop) submit2(op *Operation) (future Future) {
	channel := setupOpCh(op)
	future = channel
	op.personality = eventLoop.personality
	if eventLoop.running.Load() {
		eventLoop.ready <- op
		return
	}
	channel.Complete(0, 0, ErrCanceled)
	return
}

func (eventLoop *EventLoop) process2() {
	done := make(chan struct{})
	go eventLoop.submitAndFlushOperation(done)

	var (
		ring         = eventLoop.ring
		stopped      bool
		completed    uint32
		waitNr       uint32
		waitTimeout  *syscall.Timespec
		waitIdleTime = syscall.NsecToTimespec(eventLoop.waitIdleTimeout.Nanoseconds())
		transmission = NewCurveTransmission(eventLoop.waitTimeCurve)
		cqes         = make([]*liburing.CompletionQueueEvent, eventLoop.ring.CQEntries())
	)
	_, _ = ring.RegisterRingFd()

	waitNr, waitTimeout = transmission.Match(1)
	for {
		_, _ = ring.WaitCQEs(waitNr, waitTimeout, nil)
		if completed, stopped = eventLoop.completeCQE(&cqes); stopped {
			break
		}
		if completed == 0 {
			waitNr = 1
			waitTimeout = &waitIdleTime
			continue
		}
		if completed < waitNr {
			waitNr, waitTimeout = transmission.Down()
			continue
		}
		waitNr, waitTimeout = transmission.Up()
	}
	close(done)

}

func (eventLoop *EventLoop) submitAndFlushOperation(done <-chan struct{}) {
	var (
		wakeup     = eventLoop.group.wakeup
		ring       = eventLoop.ring
		ready      = eventLoop.ready
		readyN     uint32
		needWakeup bool
		orphan     *Operation
		stopped    bool
	)
	for {
		// wakeup
		if needWakeup {
			_ = wakeup.Wakeup(ring.Fd(), IORING_CQE_F_RING_WAKEUP)
		}

		// prepare
		if orphan != nil {
			if !eventLoop.prepareSQE(orphan) {
				continue
			}
			orphan = nil
			needWakeup = ring.FlushSQ()
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
			needWakeup = ring.FlushSQ()
			continue
		}
		// wait
		select {
		case op := <-ready:
			if !eventLoop.prepareSQE(op) {
				orphan = op
				break
			}
			needWakeup = ring.FlushSQ()
			break
		case <-done:
			stopped = true
			break
		}
		if stopped {
			break
		}
	}
	return
}

func (eventLoop *EventLoop) prepareSQE(op *Operation) bool {
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

			if cqe.UserData == 0 { // no userdata means no op or msg ring
				if cqe.Flags&IORING_CQE_F_RING_WAKEUP != 0 { // wakeup sqpoll ring
					_, _ = ring.WakeupSQPoll()
				}
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
			cop := (*Operation)(unsafe.Pointer(uintptr(cqe.UserData)))
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
			cop = nil
		}
		completed += peeked
	}
	return
}
