//go:build linux

package aio

import (
	"errors"
	"fmt"
	"github.com/brickingsoft/rio/pkg/liburing"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

func newEventLoopGroup(options Options) (group *EventLoopGroup, err error) {
	group = &EventLoopGroup{
		wakeup:     nil,
		boss:       nil,
		workers:    nil,
		workersNum: 0,
		workerIdx:  0,
	}

	// wakeup
	wakeupCh := newWakeup(group)
	wakeup := <-wakeupCh
	if err = wakeup.Valid(); err != nil {
		err = fmt.Errorf("new eventloops failed: %v", err)
		return
	}
	group.wakeup = wakeup

	// boss
	bossCh := newEventLoop(0, group, options)
	boss := <-bossCh
	if err = boss.Valid(); err != nil {
		_ = wakeup.Close()
		err = fmt.Errorf("new eventloops failed: %v", err)
		return
	}
	group.boss = boss

	// workers
	workerNum := runtime.NumCPU()/2 - 2
	if workerNum < 1 {
		workerNum = 1
	}
	workers := make([]*EventLoop, workerNum)
	for i := 0; i < workerNum; i++ {
		workerCh := newEventLoop(i+1, group, options)
		worker := <-workerCh
		if err = worker.Valid(); err != nil {
			for j := 0; j < i; j++ {
				worker = workers[j]
				_ = worker.Close()
			}
			_ = boss.Close()
			_ = wakeup.Close()
			err = fmt.Errorf("new eventloops failed: %v", err)
			return
		}
		workers[i] = worker
	}
	group.workers = workers
	group.workersNum = int64(workerNum)
	group.workerIdx = -1

	return
}

type EventLoopGroup struct {
	wakeup     *Wakeup
	boss       *EventLoop
	workers    []*EventLoop
	workersNum int64
	workerIdx  int64
}

func (group *EventLoopGroup) DispatchAndWait(sfd int, src *EventLoop) (dfd int, dst *EventLoop, err error) {
	if group.workersNum == 0 {
		dfd = sfd
		dst = group.boss
		return
	}

	idx := int64(0)
	if group.workersNum != 1 {
		idx = atomic.AddInt64(&group.workerIdx, 1) % group.workersNum
	}
	dst = group.workers[idx]
	dstFd := dst.Fd()

	op := AcquireOperation()
	op.PrepareMSGRingFd(dstFd, sfd, nil)
	dfd, _, _, err = group.boss.Submit(op).Await()
	ReleaseOperation(op)
	// close fd
	cfd := &Fd{direct: sfd, regular: -1, eventLoop: src}
	_ = cfd.Close()
	return
}

func (group *EventLoopGroup) Next() (event *EventLoop) {
	if group.workersNum == 0 {
		event = group.boss
		return
	}
	idx := int64(0)
	if group.workersNum != 1 {
		idx = atomic.AddInt64(&group.workerIdx, 1) % group.workersNum
	}
	event = group.workers[idx]
	return
}

func (group *EventLoopGroup) Submit(op *Operation) (future Future) {
	future = group.boss.Submit(op)
	return
}

func (group *EventLoopGroup) SubmitAndWait(op *Operation) (n int, flags uint32, err error) {
	n, flags, _, err = group.Submit(op).Await()
	return
}

func (group *EventLoopGroup) Cancel(target *Operation) (err error) {
	err = group.boss.Cancel(target)
	return
}

func (group *EventLoopGroup) Close() (err error) {
	for _, worker := range group.workers {
		_ = worker.Close()
	}
	_ = group.boss.Close()
	_ = group.wakeup.Close()
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
			liburing.WithFlags(liburing.IORING_SETUP_SINGLE_ISSUER),
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
		future := acquireFuture(false)
		op.future = future
		op.PrepareMSGRing(ringFd, 0)
		r.ready <- op
		_, _, _, err = future.Await()
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
			panic(errors.Join(errors.New("packing sqe failed"), err))
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
