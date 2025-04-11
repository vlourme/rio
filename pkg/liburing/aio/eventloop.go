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

func newEventLoop(id int, group *EventLoopGroup, options Options) (v <-chan *EventLoop) {
	ch := make(chan *EventLoop)
	go func(id int, group *EventLoopGroup, options Options, ch chan<- *EventLoop) {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		_ = sys.AffCPU(id)

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
		if _, ringErr = ring.RegisterRingFd(); ringErr != nil {
			_ = ring.Close()
			w := &EventLoop{
				id:  id,
				err: ringErr,
			}
			ch <- w
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
		// wait time curve
		waitTimeCurve := options.WaitCQETimeCurve

		event := &EventLoop{
			ring:           ring,
			bufferAndRings: brs,
			resource:       group.Resource(),
			group:          group,
			wg:             new(sync.WaitGroup),
			err:            nil,
			id:             id,
			key:            0,
			running:        atomic.Bool{},
			idle:           atomic.Bool{},
			idleTimeout:    waitIdleTimeout,
			waitTimeCurve:  waitTimeCurve,
			ready:          make(chan *Operation, ring.CQEntries()),
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
	ring           *liburing.Ring
	bufferAndRings *BufferAndRings
	resource       *Resource
	group          *EventLoopGroup
	wg             *sync.WaitGroup
	err            error
	id             int
	key            uint64
	running        atomic.Bool
	idle           atomic.Bool
	idleTimeout    time.Duration
	waitTimeCurve  Curve
	ready          chan *Operation
	orphan         *Operation
}

func (event *EventLoop) Id() int {
	return event.id
}

func (event *EventLoop) Key() uint64 {
	return event.key
}

func (event *EventLoop) Fd() int {
	return event.ring.Fd()
}

func (event *EventLoop) Valid() error {
	return event.err
}

func (event *EventLoop) Group() *EventLoopGroup {
	return event.group
}

func (event *EventLoop) Submit(op *Operation) {
	if event.running.Load() {
		event.ready <- op
		if event.idle.CompareAndSwap(true, false) {
			if err := event.group.wakeup.Wakeup(event.ring.Fd()); err != nil {
				op.failed(ErrCanceled)
				return
			}
		}
		return
	}
	op.failed(ErrCanceled)
	return
}

func (event *EventLoop) SubmitAndWait(op *Operation) (n int, flags uint32, err error) {
	event.Submit(op)
	n, flags, err = op.Await()
	return
}

func (event *EventLoop) Resource() *Resource {
	return event.resource
}

func (event *EventLoop) Cancel(target *Operation) (err error) {
	if target.cancelAble() {
		op := event.resource.AcquireOperation()
		op.PrepareCancel(target)
		_, _, err = event.SubmitAndWait(op)
		event.resource.ReleaseOperation(op)
	}
	return
}

func (event *EventLoop) AcquireBufferAndRing() (br *BufferAndRing, err error) {
	op := event.resource.AcquireOperation()
	op.flags |= op_f_noexec
	op.cmd = op_cmd_acquire_br
	_, _, err = event.SubmitAndWait(op)
	if err == nil {
		br = (*BufferAndRing)(op.addr)
	}
	event.resource.ReleaseOperation(op)
	return
}

func (event *EventLoop) ReleaseBufferAndRing(br *BufferAndRing) {
	event.bufferAndRings.Release(br)
}

func (event *EventLoop) Close() (err error) {
	// stop buffer and rings
	event.bufferAndRings.Stop()
	// submit close op
	op := &Operation{}
	op.PrepareCloseRing(event.key)
	event.Submit(op)
	// wait
	event.wg.Wait()
	// close ready
	close(event.ready)
	// get err
	err = event.err
	return
}

func (event *EventLoop) process() {
	event.wg.Add(1)
	defer event.wg.Done()

	event.running.Store(true)

	var (
		stopped      bool
		ready        = event.ready
		readyN       int
		completed    uint32
		waitNr       uint32
		waitTimeout  *syscall.Timespec
		idleTimeout  = syscall.NsecToTimespec(event.idleTimeout.Nanoseconds())
		ring         = event.ring
		transmission = NewCurveTransmission(event.waitTimeCurve)
		cqes         = make([]*liburing.CompletionQueueEvent, event.ring.CQEntries())
	)

	for {
		// prepare
		if event.orphan != nil {
			if !event.prepareSQE(event.orphan) {
				goto SUBMIT
			}
			event.orphan = nil
		}
		if readyN = len(ready); readyN > 0 {
			for i := 0; i < readyN; i++ {
				op := <-ready
				if !event.prepareSQE(op) {
					event.orphan = op
					goto SUBMIT
				}
			}
		}
		// submit
	SUBMIT:
		if readyN == 0 && completed == 0 { // idle
			if event.idle.CompareAndSwap(false, true) {
				waitNr, waitTimeout = 1, &idleTimeout
			} else {
				waitNr, waitTimeout = transmission.Match(1)
			}
		} else {
			waitNr, waitTimeout = transmission.Match(completed)
		}
		_, _ = ring.SubmitAndWaitTimeout(waitNr, waitTimeout, nil)
		// reset idle
		event.idle.CompareAndSwap(true, false)

		// complete
		if completed, stopped = event.completeCQE(&cqes); stopped {
			break
		}
	}

	// unregister buffer and rings
	_ = event.bufferAndRings.Unregister()
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
	event.err = event.ring.Close()
	return
}

func (event *EventLoop) prepareSQE(op *Operation) bool {
	if op.prepareAble() {
		if op.flags&op_f_noexec != 0 {
			switch op.cmd {
			case op_cmd_acquire_br:
				br, brErr := event.bufferAndRings.Acquire()
				if brErr != nil {
					op.failed(brErr)
					break
				}
				op.addr = unsafe.Pointer(br)
				op.complete(1, 0, nil)
				break
			case op_cmd_close_br:
				br := (*BufferAndRing)(op.addr)
				if err := br.free(event.ring); err != nil {
					op.failed(err)
				} else {
					op.complete(1, 0, nil)
				}
				break
			default:
				op.failed(errors.New("invalid command"))
				break
			}
			return true
		}
		// get sqe
		sqe := event.ring.GetSQE()
		if sqe == nil {
			return false
		}
		// prepare link timeout
		if op.flags&op_f_timeout != 0 {
			timeoutSQE := event.ring.GetSQE()
			if timeoutSQE == nil { // no sqe left, prepare nop and set op to head or ready
				sqe.PrepareNop()
				return false
			}
			// packing
			if err := op.packingSQE(sqe); err != nil {
				op.failed(err)
				sqe.PrepareNop()
				return true
			}
			// packing timeout
			if err := op.link.packingSQE(timeoutSQE); err != nil {
				op.failed(err)
				sqe.PrepareNop()
				return true
			}
			return true
		}
		// packing
		if err := op.packingSQE(sqe); err != nil {
			op.failed(err)
			sqe.PrepareNop()
			return true
		}
		return true
	}
	return true
}

func (event *EventLoop) completeCQE(cqesp *[]*liburing.CompletionQueueEvent) (completed uint32, stopped bool) {
	ring := event.ring
	cqes := *cqesp
	if peeked := ring.PeekBatchCQE(cqes); peeked > 0 {
		for i := uint32(0); i < peeked; i++ {
			cqe := cqes[i]
			cqes[i] = nil

			if cqe.UserData == 0 { // no userdata means no op
				ring.CQAdvance(1)
				continue
			}

			if cqe.UserData == event.key { // userdata is key means closed
				ring.CQAdvance(1)
				event.running.Store(true)
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
				opErr = os.NewSyscallError(cop.Name(), syscall.Errno(-opN))
				opN = 0
			}
			cop.complete(opN, opFlags, opErr)

			ring.CQAdvance(1)

		}
		completed += peeked
	}
	return
}
