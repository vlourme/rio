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
		brs, brsErr := newBufferAndRings(ring, options.BufferAndRingConfig)
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
			id:             id,
			key:            0,
			running:        true,
			err:            nil,
			idle:           false,
			idleTimeout:    waitIdleTimeout,
			waitTimeCurve:  waitTimeCurve,
			locker:         new(sync.Mutex),
			ready:          make([]*Operation, 0, 64),
		}
		event.key = uint64(uintptr(unsafe.Pointer(event)))

		ch <- event
		close(ch)

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
	id             int
	key            uint64
	running        bool
	err            error
	idle           bool
	idleTimeout    time.Duration
	waitTimeCurve  Curve
	locker         sync.Locker
	ready          []*Operation
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

func (event *EventLoop) Submit(op *Operation) (err error) {
	event.locker.Lock()
	if event.running {
		event.ready = append(event.ready, op)
		if event.idle {
			if err = event.group.wakeup.Wakeup(event.ring.Fd()); err != nil {
				event.ready = event.ready[:len(event.ready)-1]
				op.failed(err)
				event.locker.Unlock()
				return
			}
		}
		event.locker.Unlock()
		return
	}
	err = ErrCanceled
	event.locker.Unlock()
	return
}

func (event *EventLoop) SubmitAndWait(op *Operation) (n int, flags uint32, err error) {
	if err = event.Submit(op); err != nil {
		return
	}
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
	op := event.resource.AcquireOperation()
	op.flags |= op_f_noexec
	op.cmd = op_cmd_release_br
	op.addr = unsafe.Pointer(br)
	_, _, _ = event.SubmitAndWait(op)
	event.resource.ReleaseOperation(op)
}

func (event *EventLoop) Close() (err error) {
	// submit close op
	op := &Operation{}
	op.PrepareCloseRing(event.key)
	_ = event.Submit(op)
	// wait
	event.wg.Wait()
	// unregister buffer and rings
	_ = event.bufferAndRings.Close()
	// unregister files
	done := make(chan struct{})
	go func(ring *liburing.Ring, done chan struct{}) {
		_, _ = ring.UnregisterFiles()
		close(done)
	}(event.ring, done)
	timer := event.resource.AcquireTimer(50 * time.Millisecond)
	select {
	case <-done:
		break
	case <-timer.C:
		break
	}
	event.resource.ReleaseTimer(timer)
	// close ring
	err = event.ring.Close()
	return
}

func (event *EventLoop) process() {
	event.wg.Add(1)
	defer event.wg.Done()

	var (
		prepared     uint32
		completed    uint32
		waitNr       uint32
		waitTimeout  *syscall.Timespec
		idleTimeout  = syscall.NsecToTimespec(event.idleTimeout.Nanoseconds())
		ring         = event.ring
		transmission = NewCurveTransmission(event.waitTimeCurve)
		scratch      = make([]*Operation, 0, 64)
		cqes         = make([]*liburing.CompletionQueueEvent, 512)
	)
	for {
		// prepare
		prepared = event.prepareSQE(&scratch)
		event.locker.Lock()
		if prepared == 0 && completed == 0 && len(event.ready) == 0 { // idle
			waitNr, waitTimeout = 1, &idleTimeout
			event.idle = true
		} else {
			waitNr, waitTimeout = transmission.Match(prepared + completed)
			event.idle = false
		}
		event.locker.Unlock()
		// submit
		_, _ = ring.SubmitAndWaitTimeout(waitNr, waitTimeout, nil)
		// handle idle
		event.locker.Lock()
		if event.idle {
			event.idle = false
		}
		event.locker.Unlock()

		// complete
		completed = event.completeCQE(&cqes)

		// check running
		event.locker.Lock()
		if event.running {
			event.locker.Unlock()
			continue
		}
		event.locker.Unlock()
		break
	}
	return
}

func (event *EventLoop) prepareSQE(scratch *[]*Operation) (prepared uint32) {
	event.locker.Lock()
	readLen := len(event.ready)
	if readLen == 0 {
		event.locker.Unlock()
		return
	}
	ring := event.ring
	sqSpaceLeft := int(ring.SQSpaceLeft())
	if sqSpaceLeft == 0 {
		event.locker.Unlock()
		return
	}
	if sqSpaceLeft < readLen {
		readLen = sqSpaceLeft
	}
	*scratch = append(*scratch, event.ready[:readLen]...)
	event.ready = event.ready[readLen:]

	event.locker.Unlock()

	for i := 0; i < readLen; i++ {
		op := (*scratch)[i]
		if op.flags&op_f_noexec != 0 {
			op.prepareAble()
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
			case op_cmd_release_br:
				br := (*BufferAndRing)(op.addr)
				event.bufferAndRings.Release(br)
				op.complete(1, 0, nil)
				break
			default:
				op.failed(errors.New("invalid command"))
				break
			}
			continue
		}
		if op.prepareAble() {
			// get sqe
			sqe := ring.GetSQE()
			if op.flags&op_f_timeout != 0 { // prepare link timeout
				timeoutSQE := ring.GetSQE()
				if timeoutSQE == nil { // no sqe left, prepare nop and set op to head or ready
					event.locker.Lock()
					ready := make([]*Operation, 1, 64)
					ready[0] = op
					event.ready = append(ready, event.ready...)
					event.locker.Unlock()
					sqe.PrepareNop()
					prepared++
					return
				}
				// packing
				if err := op.packingSQE(sqe); err != nil {
					sqe.PrepareNop()
					prepared++
					panic(errors.Join(errors.New("packing sqe failed"), err))
					return
				}
				// packing timeout
				if err := op.link.packingSQE(timeoutSQE); err != nil {
					sqe.PrepareNop()
					prepared++
					panic(errors.Join(errors.New("packing sqe failed"), err))
					return
				}
				prepared += 2
				continue
			}
			// packing
			if err := op.packingSQE(sqe); err != nil {
				sqe.PrepareNop()
				prepared++
				panic(errors.Join(errors.New("packing sqe failed"), err))
				return
			}
			prepared++
		}
	}

	*scratch = (*scratch)[:0]
	return
}

func (event *EventLoop) completeCQE(cqesp *[]*liburing.CompletionQueueEvent) (completed uint32) {
	ring := event.ring
	cqes := *cqesp
	ready := int(ring.CQReady())
	for ready > 0 {
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
					event.locker.Lock()
					event.running = false
					event.locker.Unlock()
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
			ready -= int(peeked)
			completed += peeked
		}
	}

	return
}
