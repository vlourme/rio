package aio

import (
	"sync"
	"time"
)

type OperatorCanceler interface {
	Cancel()
}

var (
	operatorTimers = sync.Pool{
		New: func() interface{} {
			return &operatorTimer{
				timer:   time.NewTimer(0),
				done:    make(chan struct{}, 1),
				wg:      sync.WaitGroup{},
				locker:  new(sync.Mutex),
				stopped: false,
			}
		},
	}
)

func getOperatorTimer() *operatorTimer {
	return operatorTimers.Get().(*operatorTimer)
}

func putOperatorTimer(opTimer *operatorTimer) {
	opTimer.reset()
	operatorTimers.Put(opTimer)
}

type operatorTimer struct {
	timer            *time.Timer
	done             chan struct{}
	wg               sync.WaitGroup
	locker           sync.Locker
	stopped          bool
	deadlineExceeded bool
}

func (opt *operatorTimer) Start(timeout time.Duration, canceler OperatorCanceler) {
	opt.wg.Add(1)
	go func(opt *operatorTimer, timeout time.Duration, canceler OperatorCanceler) {
		opt.timer.Reset(timeout)
		select {
		case <-opt.done:
			break
		case <-opt.timer.C:
			opt.locker.Lock()
			if opt.stopped {
				opt.locker.Unlock()
				break
			}
			opt.stopped = true
			opt.deadlineExceeded = true
			opt.locker.Unlock()
			canceler.Cancel()
			break
		}
		opt.wg.Done()
	}(opt, timeout, canceler)
	return
}

func (opt *operatorTimer) Done() {
	// must be handled or canceled
	opt.locker.Lock()
	if opt.stopped {
		// canceled
		opt.wg.Wait()
		opt.locker.Unlock()
		return
	}
	opt.stopped = true
	opt.locker.Unlock()
	opt.done <- struct{}{}
	opt.wg.Wait()
}

func (opt *operatorTimer) reset() {
	opt.timer.Stop()
	opt.stopped = false
	opt.deadlineExceeded = false
}
