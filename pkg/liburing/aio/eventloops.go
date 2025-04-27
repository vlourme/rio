//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
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
		options.Flags = liburing.IORING_SETUP_SINGLE_ISSUER | liburing.IORING_SETUP_COOP_TASKRUN |
			liburing.IORING_SETUP_DEFER_TASKRUN | liburing.IORING_SETUP_REGISTERED_FD_ONLY
	}

	if options.Flags&liburing.IORING_SETUP_SQPOLL != 0 { // check IORING_SETUP_SQPOLL
		if cpus := uint32(runtime.NumCPU()); cpus > 1 { // IORING_SETUP_SQPOLL must be used in more than 1 cpu
			options.EventLoopCount = 1 // IORING_SETUP_SQPOLL must be one thread
			if options.SQThreadIdle == 0 {
				options.SQThreadIdle = uint32((2 * time.Second).Milliseconds())
			}
			if options.Flags&liburing.IORING_SETUP_SQ_AFF != 0 {
				if options.SQThreadCPU > cpus {
					options.SQThreadCPU = options.SQThreadCPU % cpus
				}
			}
		} else { // reset flags and count
			options.EventLoopCount = 0
			options.Flags = liburing.IORING_SETUP_SINGLE_ISSUER | liburing.IORING_SETUP_COOP_TASKRUN |
				liburing.IORING_SETUP_DEFER_TASKRUN | liburing.IORING_SETUP_REGISTERED_FD_ONLY
		}
	}

	if options.EventLoopCount == 0 {
		options.EventLoopCount = 1
	} else {
		options.EventLoopCount = liburing.FloorPow2(options.EventLoopCount)
	}

	group = &EventLoopGroup{}

	// members
	members := make([]*EventLoop, options.EventLoopCount)
	for i := uint32(0); i < options.EventLoopCount; i++ {
		memberCh := newEventLoop(int(i), group, options)
		member := <-memberCh
		if err = member.Valid(); err != nil {
			for j := uint32(0); j < i; j++ {
				_ = members[j].Close()
			}
			return
		}
		members[i] = member
	}

	group.members = members
	group.count = options.EventLoopCount
	group.mask = options.EventLoopCount - 1
	return
}

type EventLoopGroup struct {
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
	return
}

func newWakeup() (v <-chan *Wakeup) {
	ch := make(chan *Wakeup)

	go func(ch chan *Wakeup) {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		ring, ringErr := liburing.New(
			liburing.WithEntries(4),
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
		// register personality
		personality, _ := ring.RegisterPersonality()
		w := &Wakeup{
			ring:        ring,
			wg:          new(sync.WaitGroup),
			key:         0,
			personality: uint16(personality),
			ready:       make(chan *Operation, ring.SQEntries()),
			running:     atomic.Bool{},
			err:         nil,
		}
		w.key = uint64(uintptr(unsafe.Pointer(w)))

		ch <- w
		close(ch)

		w.process()
	}(ch)

	v = ch
	return
}

type Wakeup struct {
	ring        *liburing.Ring
	wg          *sync.WaitGroup
	key         uint64
	personality uint16
	ready       chan *Operation
	running     atomic.Bool
	err         error
}

func (r *Wakeup) Valid() error {
	return r.err
}

func (r *Wakeup) Wakeup(ringFd int, flags uint32) (err error) {
	if r.running.Load() {
		op := AcquireOperation()
		channel := acquireChannel(false)
		op.channel = channel
		op.personality = r.personality
		op.PrepareMSGRing(ringFd, 0, flags)
		r.ready <- op
		_, _, _, err = channel.Await()
		ReleaseOperation(op)
		return
	}
	err = ErrCanceled
	return
}

func (r *Wakeup) Close() (err error) {
	if r.running.CompareAndSwap(true, false) {
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
	}
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
		if _, swErr := ring.SubmitAndWait(1); swErr != nil {
			op.complete(0, 0, swErr)
			continue
		}
		cqe, cqeErr := ring.PeekCQE()
		if cqeErr != nil {
			op.complete(0, 0, cqeErr)
			continue
		}

		if cqe.UserData == 0 {
			ring.CQAdvance(1)
			continue
		}
		if cqe.UserData == r.key {
			ring.CQAdvance(1)
			r.running.CompareAndSwap(true, false)
			break
		}

		// get op from cqe
		cop := (*Operation)(unsafe.Pointer(uintptr(cqe.UserData)))
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
		cop = nil
	}
	// close ring
	r.err = r.ring.Close()
}
