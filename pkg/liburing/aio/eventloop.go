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

		affCPU := id
		// options
		opts := make([]liburing.Option, 0, 1)
		opts = append(opts, liburing.WithEntries(options.Entries))
		opts = append(opts, liburing.WithFlags(options.Flags))
		if options.Flags&liburing.IORING_SETUP_SQPOLL != 0 {
			opts = append(opts, liburing.WithSQThreadIdle(options.SQThreadIdle))
			if options.Flags&liburing.IORING_SETUP_SQ_AFF != 0 {
				opts = append(opts, liburing.WithSQThreadCPU(options.SQThreadCPU))
				cpus := runtime.NumCPU()
				affCPU = (int(options.SQThreadCPU) + 1) % cpus
				if affCPU == int(options.SQThreadCPU) {
					affCPU = -1
				}
			}
		}
		// aff cpu
		if affCPU > -1 {
			_ = sys.AffCPU(affCPU)
		}
		// new ring
		ring, ringErr := liburing.New(opts...)
		if ringErr != nil {
			event := &EventLoop{
				id:  id,
				err: ringErr,
			}
			ch <- event
			close(ch)
			return
		}
		// wakeup
		var wakeup *Wakeup
		if ring.Flags()&liburing.IORING_SETUP_SINGLE_ISSUER != 0 {
			wakeupCh := newWakeup()
			wakeup = <-wakeupCh
			if wakeupErr := wakeup.Valid(); wakeupErr != nil {
				_ = ring.Close()
				event := &EventLoop{
					id:  id,
					err: wakeupErr,
				}
				ch <- event
				close(ch)
				return
			}
		}
		// register personality
		personality, _ := ring.RegisterPersonality()
		// register napi
		var napi *liburing.NAPI
		if options.NAPIBusyPollTimeout > 0 {
			us := uint32(options.NAPIBusyPollTimeout.Microseconds())
			if us == 0 {
				us = 50
			}
			napi = &liburing.NAPI{
				BusyPollTo:     us,
				PreferBusyPoll: 1,
				Resv:           0,
			}
			_, napiErr := ring.RegisterNAPI(napi)
			if napiErr != nil {
				_ = ring.Close()
				event := &EventLoop{
					id:  id,
					err: napiErr,
				}
				ch <- event
				close(ch)
				return
			}
		}

		// buffer and ring group
		bufferAndRingGroup, bufferAndRingGroupErr := newBufferAndRingGroup(options.BufferAndRingConfig)
		if bufferAndRingGroupErr != nil {
			_ = ring.Close()
			event := &EventLoop{
				id:  id,
				err: bufferAndRingGroupErr,
			}
			ch <- event
			close(ch)
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
			close(ch)
			return
		}

		eventLoop := &EventLoop{
			ring:               ring,
			wakeup:             wakeup,
			bufferAndRingGroup: bufferAndRingGroup,
			group:              group,
			wg:                 new(sync.WaitGroup),
			id:                 id,
			key:                uint64(uintptr(unsafe.Pointer(ring))),
			personality:        uint16(personality),
			running:            atomic.Bool{},
			idle:               atomic.Bool{},
			waitTimeoutCurve:   options.WaitCQETimeoutCurve,
			ready:              make(chan *Operation, ring.SQEntries()),
			err:                nil,
		}
		eventLoop.running.Store(true)
		// return event loop
		ch <- eventLoop
		close(ch)

		eventLoop.wg.Add(1)
		// start buffer and ring group loop
		bufferAndRingGroup.Start(eventLoop)
		// process
		eventLoop.process()

		// shutdown >>>
		// unregister buffer and ring group
		_ = bufferAndRingGroup.Unregister()
		// unregister napi
		if napi != nil {
			_, _ = ring.UnregisterNAPI(napi)
		}
		// unregister personality
		_, _ = ring.UnregisterPersonality()
		// unregister files
		_, _ = ring.UnregisterFiles()

		// close ring
		eventLoop.err = ring.Close()

		// close ready
		for i := 0; i < len(eventLoop.ready); i++ {
			op := <-eventLoop.ready
			op.complete(0, 0, ErrCanceled)
		}
		close(eventLoop.ready)

		// close wakeup
		if wakeup != nil {
			_ = wakeup.Close()
		}
		// shutdown <<<

		eventLoop.wg.Done()
	}(id, group, options, ch)
	v = ch
	return
}

type EventLoop struct {
	ring               *liburing.Ring
	wakeup             *Wakeup
	bufferAndRingGroup *BufferAndRingGroup
	group              *EventLoopGroup
	wg                 *sync.WaitGroup
	id                 int
	key                uint64
	personality        uint16
	running            atomic.Bool
	idle               atomic.Bool
	waitTimeoutCurve   Curve
	submitter          func(op *Operation)
	ready              chan *Operation
	orphan             *Operation
	err                error
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
	channel := eventLoop.setupChannel(op)
	future = channel
	if eventLoop.running.Load() {
		eventLoop.submitter(op)
	} else {
		channel.Complete(0, 0, ErrCanceled)
	}
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
	br, err = eventLoop.bufferAndRingGroup.Acquire()
	return
}

func (eventLoop *EventLoop) ReleaseBufferAndRing(br *BufferAndRing) {
	eventLoop.bufferAndRingGroup.Release(br)
}

func (eventLoop *EventLoop) Close() (err error) {
	if eventLoop.running.CompareAndSwap(true, false) {
		// stop buffer and rings
		eventLoop.bufferAndRingGroup.Stop()
		// submit close op
		op := &Operation{}
		op.PrepareCloseRing(eventLoop.key)
		eventLoop.submitter(op)
		// wait
		eventLoop.wg.Wait()
		// get err
		err = eventLoop.err
	}
	return
}

func (eventLoop *EventLoop) process() {
	if eventLoop.ring.Flags()&liburing.IORING_SETUP_SINGLE_ISSUER == 0 {
		eventLoop.submitter = eventLoop.submit2
		eventLoop.process2()
	} else {
		eventLoop.submitter = eventLoop.submit1
		eventLoop.process1()
	}
}

func (eventLoop *EventLoop) setupChannel(op *Operation) *Channel {
	if op.channel != nil {
		return op.channel
	}
	channel := acquireChannel(op.kind == op_kind_multishot)
	if op.timeout != nil {
		op.timeout.channel = acquireChannel(false)
		channel.timeout = op.timeout.channel
	}
	op.channel = channel
	return channel
}

func (eventLoop *EventLoop) submit1(op *Operation) {
	eventLoop.ready <- op
	if eventLoop.idle.CompareAndSwap(true, false) {
		if err := eventLoop.wakeup.Wakeup(eventLoop.ring.Fd(), 0); err != nil {
			op.complete(0, 0, ErrCanceled)
			return
		}
	}
	return
}

func (eventLoop *EventLoop) process1() {
	curve := eventLoop.waitTimeoutCurve
	if len(curve) == 0 {
		curve = SCurve
	}
	if curve.HasNoTimeout() {
		curve = SCurve
	}
	var (
		stopped      bool
		ready        = eventLoop.ready
		readyN       uint32
		registerN    uint32
		completed    uint32
		waitNr       uint32
		waitTimeout  *syscall.Timespec
		ring         = eventLoop.ring
		transmission = NewCurveTransmission(curve)
		cqes         = make([]*liburing.CompletionQueueEvent, eventLoop.ring.CQEntries())
	)

	// register ring
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
				if op.kind == op_kind_register {
					registerN++
				}
				if !eventLoop.prepareSQE(op) {
					eventLoop.orphan = op
					break
				}
			}
		}
		readyN -= registerN
		registerN = 0
		// submit and wait
	SUBMIT:
		if readyN == 0 && completed == 0 { // idle
			if eventLoop.idle.CompareAndSwap(false, true) {
				waitNr, waitTimeout = 1, nil
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

func (eventLoop *EventLoop) submit2(op *Operation) {
	eventLoop.ready <- op
	return
}

func (eventLoop *EventLoop) process2() {
	wg := new(sync.WaitGroup)
	done := make(chan struct{})
	wg.Add(1)
	go func(eventLoop *EventLoop, wg *sync.WaitGroup, done chan struct{}) {
		var (
			ring    = eventLoop.ring
			ready   = eventLoop.ready
			readyN  uint32
			orphan  *Operation
			stopped bool
		)
		for {
			// orphan
			if orphan != nil {
				if !eventLoop.prepareSQE(orphan) {
					_, _ = ring.Submit()
					continue
				}
				orphan = nil
				_, _ = ring.Submit()
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
		wg.Done()
	}(eventLoop, wg, done)

	var (
		ring         = eventLoop.ring
		stopped      bool
		completed    uint32
		waitNr       uint32            = 1
		waitTimeout  *syscall.Timespec = nil
		transmission                   = NewCurveTransmission(eventLoop.waitTimeoutCurve)
		cqes                           = make([]*liburing.CompletionQueueEvent, eventLoop.ring.CQEntries())
	)

	for {
		_, _ = ring.WaitCQEs(waitNr, waitTimeout, nil)
		if completed, stopped = eventLoop.completeCQE(&cqes); stopped {
			break
		}
		if completed == 0 {
			waitNr = 1
			waitTimeout = nil
			continue
		}
		if completed < waitNr {
			waitNr, waitTimeout = transmission.Down()
			continue
		}
		waitNr, waitTimeout = transmission.Up()
	}
	close(done)
	wg.Wait()
}

func (eventLoop *EventLoop) handleRegister(op *Operation) bool {
	if op.kind == op_kind_register {
		switch op.cmd {
		case op_cmd_register_buffer_and_ring:
			r := (*BufferAndRingRegister)(op.addr)
			op.channel.adaptor = r
			op.complete(0, 0, nil)
			break
		case op_cmd_unregister_buffer_and_ring:
			r := (*BufferAndRingUnregister)(op.addr)
			op.channel.adaptor = r
			op.complete(0, 0, nil)
			break
		default:
			op.complete(0, 0, errors.New("invalid register cmd"))
			break
		}
		return true
	}
	return false
}

func (eventLoop *EventLoop) prepareSQE(op *Operation) bool {
	if eventLoop.handleRegister(op) {
		return true
	}
	// get sqe
	sqe := eventLoop.ring.GetSQE()
	if sqe == nil {
		return false
	}
	op.personality = eventLoop.personality
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
		op.timeout.personality = eventLoop.personality
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

			if cqe.UserData == 0 { // no op
				ring.CQAdvance(1)
				continue
			}

			if cqe.UserData == eventLoop.key { // userdata is key means closed
				ring.CQAdvance(1)
				eventLoop.running.CompareAndSwap(true, false)
				stopped = true
				continue
			}

			// get op from cqe
			copp := unsafe.Pointer(uintptr(cqe.UserData))
			cop := (*Operation)(copp)
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
			copp = nil
			cqe.UserData = 0
		}
		completed += peeked
	}
	return
}
