package sockets

import (
	"sync"
	"time"
)

const (
	accept OperationMode = iota + 1
	dial
	read
	write
	readFrom
	writeTo
	readMsg
	writeMsg
)

type OperationMode int

func (op OperationMode) String() string {
	switch op {
	case accept:
		return "accept"
	case dial:
		return "dial"
	case read:
		return "read"
	case write:
		return "write"
	case readFrom:
		return "readFrom"
	case writeTo:
		return "writeTo"
	case readMsg:
		return "readMsg"
	case writeMsg:
		return "writeMsg"
	default:
		return "unknown"
	}
}

type OperationCanceler interface {
	Cancel()
}

var (
	operationTimers = sync.Pool{
		New: func() interface{} {
			return &operationTimer{
				timer:   time.NewTimer(0),
				done:    make(chan struct{}, 1),
				wg:      sync.WaitGroup{},
				locker:  new(sync.Mutex),
				stopped: false,
			}
		},
	}
)

func getOperationTimer() *operationTimer {
	return operationTimers.Get().(*operationTimer)
}

func putOperationTimer(opTimer *operationTimer) {
	opTimer.reset()
	operationTimers.Put(opTimer)
}

type operationTimer struct {
	timer   *time.Timer
	done    chan struct{}
	wg      sync.WaitGroup
	locker  sync.Locker
	stopped bool
}

func (opt *operationTimer) Start(timeout time.Duration, canceler OperationCanceler) {
	opt.wg.Add(1)
	go func(opt *operationTimer, timeout time.Duration, canceler OperationCanceler) {
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
			opt.locker.Unlock()
			canceler.Cancel()
			break
		}
		opt.wg.Done()
	}(opt, timeout, canceler)
	return
}

func (opt *operationTimer) Done() {
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

func (opt *operationTimer) reset() {
	opt.timer.Stop()
	opt.stopped = false
}
