package async

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

type TaskSubmitter interface {
	Submit(task func())
}

type Executors interface {
	TryExecute(ctx context.Context, task func()) (ok bool)
	Execute(ctx context.Context, task func()) (err error)
	TryGetTaskSubmitter() (submitter TaskSubmitter, has bool)
	ReleaseNotUsedTaskSubmitter(submitter TaskSubmitter)
	Close()
	CloseGracefully()
}

type submitterImpl struct {
	lastUseTime time.Time
	ch          chan func()
}

func (submitter *submitterImpl) Submit(task func()) {
	submitter.ch <- task
}

const (
	ns500 = 500 * time.Nanosecond
)

type counter struct {
	n int64
}

func (c *counter) Incr() int64 {
	return atomic.AddInt64(&c.n, 1)
}

func (c *counter) Decr() int64 {
	return atomic.AddInt64(&c.n, -1)
}

func (c *counter) Value() int64 {
	return atomic.LoadInt64(&c.n)
}

func (c *counter) Wait() {
	times := 10
	for {
		n := c.Value()
		if n < 1 {
			break
		}
		time.Sleep(ns500)
		times--
		if times < 1 {
			times = 10
			runtime.Gosched()
		}
	}
}

func New(options ...Option) Executors {
	opt := Options{
		MaxGoroutines:            defaultMaxGoroutines,
		MaxGoroutineIdleDuration: defaultMaxGoroutineIdleDuration,
		CloseTimeout:             0,
	}
	if options != nil {
		for _, option := range options {
			optErr := option(&opt)
			if optErr != nil {
				panic(fmt.Errorf("async: new executors failed, %v", optErr))
				return nil
			}
		}
	}
	exec := &executors{
		maxGoroutines:            int64(opt.MaxGoroutines),
		maxGoroutineIdleDuration: opt.MaxGoroutineIdleDuration,
		locker:                   new(sync.Mutex),
		running:                  0,
		ready:                    nil,
		submitters:               sync.Pool{},
		goroutines:               new(counter),
		stopCh:                   nil,
		stopTimeout:              0,
	}
	exec.start()
	return exec
}

type executors struct {
	maxGoroutines            int64
	maxGoroutineIdleDuration time.Duration
	locker                   sync.Locker
	running                  int64
	ready                    []*submitterImpl
	submitters               sync.Pool
	goroutines               *counter
	stopCh                   chan struct{}
	stopTimeout              time.Duration
}

func (exec *executors) TryExecute(ctx context.Context, task func()) (ok bool) {
	if task == nil || atomic.LoadInt64(&exec.running) == 0 {
		return false
	}
	submitter := exec.getSubmitter()
	if submitter == nil {
		return false
	}
	select {
	case <-ctx.Done():
		exec.ReleaseNotUsedTaskSubmitter(submitter)
		break
	default:
		submitter.Submit(task)
		ok = true
		break
	}

	return
}

var (
	ErrExecutorsWasClosed = errors.New("async: executors were closed")
)

func (exec *executors) Execute(ctx context.Context, task func()) (err error) {
	if task == nil || atomic.LoadInt64(&exec.running) == 0 {
		return
	}
	times := 10
	for {
		ok := exec.TryExecute(ctx, task)
		if ok {
			break
		}
		if err = ctx.Err(); err != nil {
			break
		}
		if atomic.LoadInt64(&exec.running) == 0 {
			err = ErrExecutorsWasClosed
			return
		}
		time.Sleep(ns500)
		times--
		if times < 0 {
			times = 10
			runtime.Gosched()
		}
	}
	return
}

func (exec *executors) TryGetTaskSubmitter() (submitter TaskSubmitter, has bool) {
	submitter = exec.getSubmitter()
	has = submitter != nil
	return
}

func (exec *executors) ReleaseNotUsedTaskSubmitter(submitter TaskSubmitter) {
	exec.release(submitter.(*submitterImpl))
	return
}

func (exec *executors) shutdown() {
	close(exec.stopCh)
	exec.locker.Lock()
	ready := exec.ready
	for i := range ready {
		ready[i].ch <- nil
		ready[i] = nil
	}
	exec.ready = ready[:0]
	exec.locker.Unlock()
}

func (exec *executors) Close() {
	if exec.stopTimeout > 0 {
		exec.CloseGracefully()
		return
	}
	atomic.StoreInt64(&exec.running, 0)
	exec.shutdown()
}

func (exec *executors) CloseGracefully() {
	atomic.StoreInt64(&exec.running, 0)
	exec.shutdown()
	if exec.stopTimeout == 0 {
		exec.goroutines.Wait()
		return
	}
	ch := make(chan struct{}, 1)
	go func(exec *executors, ch chan struct{}) {
		exec.goroutines.Wait()
		close(ch)
	}(exec, ch)
	timer := time.NewTimer(exec.stopTimeout)
	select {
	case <-timer.C:
		break
	case <-ch:
		break
	}
	timer.Stop()
}

func (exec *executors) start() {
	exec.running = 1
	exec.stopCh = make(chan struct{})
	exec.submitters.New = func() interface{} {
		return &submitterImpl{
			ch: make(chan func(), 1),
		}
	}
	go func(exec *executors) {
		var scratch []*submitterImpl
		maxExecutorIdleDuration := exec.maxGoroutineIdleDuration
		stopped := false
		timer := time.NewTimer(maxExecutorIdleDuration)
		for {
			select {
			case <-exec.stopCh:
				stopped = true
				break
			case <-timer.C:
				exec.clean(&scratch)
				timer.Reset(maxExecutorIdleDuration)
				break
			}
			if stopped {
				break
			}
		}
		timer.Stop()
	}(exec)
}

func (exec *executors) clean(scratch *[]*submitterImpl) {
	if atomic.LoadInt64(&exec.running) == 0 {
		return
	}
	maxExecutorIdleDuration := exec.maxGoroutineIdleDuration
	criticalTime := time.Now().Add(-maxExecutorIdleDuration)
	exec.locker.Lock()
	ready := exec.ready
	n := len(ready)
	l, r, mid := 0, n-1, 0
	for l <= r {
		mid = (l + r) / 2
		if criticalTime.After(exec.ready[mid].lastUseTime) {
			l = mid + 1
		} else {
			r = mid - 1
		}
	}
	i := r
	if i == -1 {
		exec.locker.Unlock()
		return
	}
	*scratch = append((*scratch)[:0], ready[:i+1]...)
	m := copy(ready, ready[i+1:])
	for i = m; i < n; i++ {
		ready[i] = nil
	}
	exec.ready = ready[:m]
	exec.locker.Unlock()

	tmp := *scratch
	for iot := range tmp {
		tmp[iot].ch <- nil
		tmp[iot] = nil
	}
}

func (exec *executors) getSubmitter() *submitterImpl {
	var submitter *submitterImpl
	createExecutor := false
	exec.locker.Lock()
	ready := exec.ready
	n := len(ready) - 1
	if n < 0 {
		if exec.goroutines.Value() < exec.maxGoroutines {
			createExecutor = true
			exec.goroutines.Incr()
		}
	} else {
		submitter = ready[n]
		ready[n] = nil
		exec.ready = ready[:n]
	}
	exec.locker.Unlock()
	if submitter == nil {
		if !createExecutor {
			return nil
		}
		vch := exec.submitters.Get()
		submitter = vch.(*submitterImpl)
		go func(exec *executors) {
			exec.handle(submitter)
			exec.submitters.Put(vch)
		}(exec)
	}
	return submitter
}

func (exec *executors) release(submitter *submitterImpl) bool {
	submitter.lastUseTime = time.Now()
	exec.locker.Lock()
	if atomic.LoadInt64(&exec.running) == 0 {
		exec.locker.Unlock()
		return false
	}
	exec.ready = append(exec.ready, submitter)
	exec.locker.Unlock()
	return true
}

func (exec *executors) handle(wch *submitterImpl) {
	for {
		if wch == nil {
			break
		}
		task, ok := <-wch.ch
		if !ok {
			break
		}
		if task == nil {
			break
		}
		task()
		if !exec.release(wch) {
			break
		}
	}
	exec.locker.Lock()
	exec.goroutines.Decr()
	exec.locker.Unlock()
}
