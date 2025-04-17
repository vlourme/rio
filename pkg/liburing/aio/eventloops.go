//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

func newEventLoopGroup(options Options) (group *EventLoopGroup, err error) {

	if options.Flags == 0 { // set default flags
		options.Flags = liburing.IORING_SETUP_COOP_TASKRUN |
			liburing.IORING_SETUP_SINGLE_ISSUER |
			liburing.IORING_SETUP_TASKRUN_FLAG
		options.Flags = liburing.IORING_SETUP_COOP_TASKRUN | liburing.IORING_SETUP_SINGLE_ISSUER
	}

	if options.Flags&liburing.IORING_SETUP_SQPOLL != 0 { // check IORING_SETUP_SQPOLL
		options.EventLoopCount = 1 // IORING_SETUP_SQPOLL must one thread
		if options.Flags&liburing.IORING_SETUP_SQ_AFF != 0 {
			_ = sys.MaskCPU(int(options.SQThreadCPU))
		}
	}
	if options.EventLoopCount == 0 { // build count
		var dividend uint32
		if options.Flags&liburing.IORING_SETUP_SINGLE_ISSUER != 0 {
			dividend = 2 // IORING_SETUP_SINGLE_ISSUER means one ring one thread, so 1/2.
		} else {
			dividend = 4 // others means one ring two thread, so 1/4.
		}
		options.EventLoopCount = liburing.FloorPow2(uint32(runtime.NumCPU()) / dividend)
		if options.EventLoopCount == 0 {
			options.EventLoopCount = 1
		}
	}
	// auto 						// 27000
	//options.EventLoopCount = 1 	// 38000

	/* c1 38000
	{1, 10 * time.Microsecond},
			{2, 20 * time.Microsecond},
			{4, 40 * time.Microsecond},
			{8, 50 * time.Microsecond},
			{16, 100 * time.Microsecond},
			{32, 200 * time.Microsecond},
			{64, 300 * time.Microsecond},
			{128, 400 * time.Microsecond},
			{512, 500 * time.Microsecond},
	*/

	if options.WaitCQEIdleTimeout < time.Second { // min wait cqe idle timeout is 1 sec
		options.WaitCQEIdleTimeout = 15 * time.Second // default is 15 sec
	}

	if len(options.WaitCQETimeCurve) == 0 {
		options.WaitCQETimeCurve = Curve{
			//{4, 2 * time.Microsecond},
			//{8, 5 * time.Microsecond},
			//{16, 10 * time.Microsecond},
			//{32, 15 * time.Microsecond},
			//{64, 20 * time.Microsecond},
			{2, 10 * time.Microsecond},
			{4, 20 * time.Microsecond},
			{8, 50 * time.Microsecond},
			{16, 100 * time.Microsecond},
			{32, 150 * time.Microsecond},
			{64, 300 * time.Microsecond},
			{128, 400 * time.Microsecond},
			{512, 500 * time.Microsecond},
		}
	}

	group = &EventLoopGroup{}

	// wakeup
	var wakeup *Wakeup
	if options.Flags&liburing.IORING_SETUP_SINGLE_ISSUER != 0 {
		wakeupCh := newWakeup(group)
		wakeup = <-wakeupCh
		if err = wakeup.Valid(); err != nil {
			return
		}
		group.wakeup = wakeup
	}

	// members
	var (
		members []*EventLoop
	)

	members = make([]*EventLoop, options.EventLoopCount)
	for i := uint32(0); i < options.EventLoopCount; i++ {
		memberCh := newEventLoop(int(i), group, options)
		member := <-memberCh
		if err = member.Valid(); err != nil {
			if wakeup != nil {
				_ = wakeup.Close()
			}
			for j := uint32(0); j < i; j++ {
				_ = members[j].Close()
			}
			return
		}
		members[i] = member
	}

	group.wakeup = wakeup
	group.members = members
	group.count = options.EventLoopCount
	group.mask = options.EventLoopCount - 1

	return
}

type EventLoopGroup struct {
	wakeup  *Wakeup
	members []*EventLoop
	count   uint32
	mask    uint32
	index   atomic.Uint32
}

func (group *EventLoopGroup) Dispatch(srcFd int, srcEventLoop *EventLoop) (dstFd int, dstEventLoop *EventLoop, err error) {
	if group.count == 1 {
		dstFd = srcFd
		dstEventLoop = srcEventLoop
		return
	}

	index := group.index.Add(1) & group.mask
	member := group.members[index]

	if srcEventLoop.Fd() == member.Fd() {
		dstFd = srcFd
		dstEventLoop = srcEventLoop
		return
	}

	dstEventLoop = member

	op := AcquireOperation()
	op.PrepareMSGRingFd(dstEventLoop.Fd(), srcFd, nil)
	dstFd, _, err = srcEventLoop.SubmitAndWait(op)
	ReleaseOperation(op)
	// close fd
	op = AcquireOperation()
	op.PrepareCloseDirect(srcFd)
	_, _, _ = srcEventLoop.SubmitAndWait(op)
	ReleaseOperation(op)
	return
}

func (group *EventLoopGroup) Next() (event *EventLoop) {
	if group.count == 1 {
		event = group.members[0]
		return
	}
	index := group.index.Add(1) & group.mask
	event = group.members[index]
	return
}

func (group *EventLoopGroup) Close() (err error) {
	for _, member := range group.members {
		_ = member.Close()
	}
	if group.wakeup != nil {
		_ = group.wakeup.Close()
	}
	return
}

func newWakeup(group *EventLoopGroup) (v <-chan *Wakeup) {
	ch := make(chan *Wakeup)

	go func(group *EventLoopGroup, ch chan *Wakeup) {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		entries := runtime.NumCPU() * 2
		ring, ringErr := liburing.New(
			liburing.WithEntries(uint32(entries)),
			liburing.WithFlags(liburing.IORING_SETUP_COOP_TASKRUN|liburing.IORING_SETUP_SINGLE_ISSUER),
		)
		if ringErr != nil {
			w := &Wakeup{
				err: ringErr,
			}
			ch <- w
			return
		}
		if _, ringErr = ring.RegisterRingFd(); ringErr != nil {
			_ = ring.Close()
			w := &Wakeup{
				err: ringErr,
			}
			ch <- w
			return
		}
		w := &Wakeup{
			ring:    ring,
			group:   group,
			wg:      new(sync.WaitGroup),
			key:     0,
			ready:   make(chan *Operation, ring.SQEntries()),
			running: atomic.Bool{},
			err:     nil,
		}
		w.key = uint64(uintptr(unsafe.Pointer(w)))

		ch <- w
		close(ch)

		w.process()
	}(group, ch)

	v = ch
	return
}

type Wakeup struct {
	ring    *liburing.Ring
	group   *EventLoopGroup
	wg      *sync.WaitGroup
	key     uint64
	ready   chan *Operation
	running atomic.Bool
	err     error
}

func (r *Wakeup) Valid() error {
	return r.err
}

func (r *Wakeup) Wakeup(ringFd int) (err error) {
	if r.running.Load() {
		op := AcquireOperation()
		channel := acquireChannel(false)
		op.channel = channel
		op.PrepareMSGRing(ringFd, 0)
		r.ready <- op
		_, _, _, err = channel.Await()
		ReleaseOperation(op)
		return
	}
	err = ErrCanceled
	return
}

func (r *Wakeup) Close() (err error) {
	// submit close op
	op := &Operation{}
	op.PrepareCloseRing(r.key)
	r.ready <- op
	// wait
	r.wg.Wait()
	// close ready
	close(r.ready)
	// get err
	err = r.err
	return
}

func (r *Wakeup) process() {
	r.wg.Add(1)
	defer r.wg.Done()

	r.running.Store(true)

	ring := r.ring
	for {
		op, ok := <-r.ready
		if !ok {
			return
		}
		sqe := ring.GetSQE()
		if sqe == nil {
			_, _ = ring.Submit()
			sqe = ring.GetSQE()
		}
		if err := op.packingSQE(sqe); err != nil {
			panic(errors.Join(errors.New("packing sqe failed"), err, errors.New(op.Name())))
			return
		}
		_, _ = ring.SubmitAndWait(1)
		cqe, _ := ring.PeekCQE()

		if cqe.UserData == 0 {
			ring.CQAdvance(1)
			continue
		}
		if cqe.UserData == r.key {
			ring.CQAdvance(1)
			r.running.Store(false)
			break
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
		}
		cop.complete(opN, opFlags, opErr)

		ring.CQAdvance(1)
	}
	// close ring
	r.err = r.ring.Close()
}
