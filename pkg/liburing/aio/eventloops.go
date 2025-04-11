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
		resource:   new(Resource),
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
	resource   *Resource
	wakeup     *Wakeup
	boss       *EventLoop
	workers    []*EventLoop
	workersNum int64
	workerIdx  int64
}

func (group *EventLoopGroup) Dispatch(fd int, attached *Operation) (err error) {
	idx := int64(0)
	if group.workersNum != 1 {
		idx = atomic.AddInt64(&group.workerIdx, 1) % group.workersNum
	}
	work := group.workers[idx]
	attached.addr = unsafe.Pointer(work)
	efd := work.Fd()

	op := group.boss.resource.AcquireOperation()
	op.PrepareMSGRingFd(efd, fd, attached)
	group.boss.Submit(op)
	_, _, err = op.Await()
	group.boss.resource.ReleaseOperation(op)
	return
}

func (group *EventLoopGroup) DispatchAndWait(fd int) (dfd int, worker *EventLoop, err error) {
	if group.workersNum == 0 {
		dfd = fd
		worker = group.boss
		return
	}

	idx := int64(0)
	if group.workersNum != 1 {
		idx = atomic.AddInt64(&group.workerIdx, 1) % group.workersNum
	}
	worker = group.workers[idx]
	efd := worker.Fd()
	op := group.boss.resource.AcquireOperation()
	op.PrepareMSGRingFd(efd, fd, nil)
	group.boss.Submit(op)
	dfd, _, err = op.Await()
	group.boss.resource.ReleaseOperation(op)
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

func (group *EventLoopGroup) Submit(op *Operation) {
	group.boss.Submit(op)
	return
}

func (group *EventLoopGroup) SubmitAndWait(op *Operation) (n int, flags uint32, err error) {
	group.Submit(op)
	n, flags, err = op.Await()
	return
}

func (group *EventLoopGroup) Cancel(target *Operation) (err error) {
	err = group.boss.Cancel(target)
	return
}

func (group *EventLoopGroup) Resource() *Resource {
	return group.resource
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
			ring:     ring,
			resource: group.Resource(),
			wg:       new(sync.WaitGroup),
			key:      0,
			ready:    make(chan *Operation, ring.SQEntries()),
			running:  atomic.Bool{},
			err:      nil,
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
	ring     *liburing.Ring
	resource *Resource
	wg       *sync.WaitGroup
	key      uint64
	ready    chan *Operation
	running  atomic.Bool
	err      error
}

func (r *Wakeup) Valid() error {
	return r.err
}

func (r *Wakeup) Wakeup(ringFd int) (err error) {
	if r.running.Load() {
		op := r.resource.AcquireOperation()
		op.PrepareMSGRing(ringFd, 0)
		r.ready <- op
		_, _, err = op.Await()
		r.resource.ReleaseOperation(op)
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
		op.prepareAble()
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
