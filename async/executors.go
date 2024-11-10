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

type Runnable interface {
	Run(ctx context.Context)
}

type runnableFunc struct {
	fn func(ctx context.Context)
}

func (exec *runnableFunc) Run(ctx context.Context) {
	exec.fn(ctx)
}

func RunnableFunc(fn func(ctx context.Context)) Runnable {
	return &runnableFunc{
		fn: fn,
	}
}

type ExecutorSubmitter interface {
	Submit(ctx context.Context, runnable Runnable)
}

type Executors interface {
	TryExecute(ctx context.Context, runnable Runnable) (ok bool)
	Execute(ctx context.Context, runnable Runnable) (err error)
	GetExecutorSubmitter() (submitter ExecutorSubmitter, has bool)
	ReleaseNotUsedExecutorSubmitter(submitter ExecutorSubmitter)
	Available() (ok bool)
	Close()
	CloseGracefully()
}

type executor struct {
	ctx      context.Context
	runnable Runnable
}

type executorSubmitterImpl struct {
	lastUseTime time.Time
	ch          chan *executor
}

func (submitter *executorSubmitterImpl) Submit(ctx context.Context, runnable Runnable) {
	submitter.ch <- &executor{
		ctx:      ctx,
		runnable: runnable,
	}
}

const (
	ns500 = 500 * time.Nanosecond
)

func New(options ...Option) Executors {
	opt := Options{
		MaxExecutors:            defaultMaxExecutors,
		MaxExecutorIdleDuration: defaultMaxExecutorIdleDuration,
	}
	if options != nil {
		for _, option := range options {
			optErr := option(&opt)
			if optErr != nil {
				panic(fmt.Errorf("rio: new executors failed, %v", optErr))
				return nil
			}
		}
	}
	exec := &executors{
		maxExecutorsCount:       int64(opt.MaxExecutors),
		maxExecutorIdleDuration: opt.MaxExecutorIdleDuration,
		locker:                  sync.Mutex{},
		running:                 0,
		mustStop:                false,
		ready:                   nil,
		stopCh:                  nil,
		submitters:              sync.Pool{},
		submittersWaitGroup:     new(sync.WaitGroup),
		goroutines:              0,
		goroutinesWaitGroup:     new(sync.WaitGroup),
	}
	exec.start()
	return exec
}

type executors struct {
	maxExecutorsCount       int64
	maxExecutorIdleDuration time.Duration
	locker                  sync.Mutex
	running                 int64
	mustStop                bool
	ready                   []*executorSubmitterImpl
	stopCh                  chan struct{}
	submitters              sync.Pool
	submittersWaitGroup     *sync.WaitGroup
	goroutines              int64
	goroutinesWaitGroup     *sync.WaitGroup
}

func (exec *executors) TryExecute(ctx context.Context, runnable Runnable) (ok bool) {
	if runnable == nil || atomic.LoadInt64(&exec.running) == 0 {
		return false
	}
	submitter := exec.getSubmitter()
	if submitter == nil {
		return false
	}
	submitter.Submit(ctx, runnable)
	ok = true
	return
}

var (
	ErrExecutorsWasClosed = errors.New("executors were closed")
)

func (exec *executors) Execute(ctx context.Context, runnable Runnable) (err error) {
	if runnable == nil || atomic.LoadInt64(&exec.running) == 0 {
		return
	}
	times := 10
	for {
		ok := exec.TryExecute(ctx, runnable)
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

func (exec *executors) GetExecutorSubmitter() (submitter ExecutorSubmitter, has bool) {
	submitter = exec.getSubmitter()
	has = submitter != nil
	return
}

func (exec *executors) ReleaseNotUsedExecutorSubmitter(submitter ExecutorSubmitter) {
	exec.release(submitter.(*executorSubmitterImpl))
	return
}

func (exec *executors) Available() (ok bool) {
	exec.locker.Lock()
	if n := len(exec.ready) - 1; n < 0 {
		if exec.goroutines < exec.maxExecutorsCount {
			ok = true
		}
	} else {
		ok = true
	}
	exec.locker.Unlock()
	return
}

func (exec *executors) shutdown() {
	close(exec.stopCh)
	exec.stopCh = nil
	exec.locker.Lock()
	ready := exec.ready
	for i := range ready {
		ready[i].ch <- nil
		ready[i] = nil
	}
	exec.ready = ready[:0]
	exec.mustStop = true
	exec.locker.Unlock()
}

func (exec *executors) Close() {
	atomic.StoreInt64(&exec.running, 0)
	exec.shutdown()
}

func (exec *executors) CloseGracefully() {
	atomic.StoreInt64(&exec.running, 0)
	exec.submittersWaitGroup.Wait()
	exec.shutdown()
	exec.goroutinesWaitGroup.Wait()
}

func (exec *executors) start() {
	exec.running = 1
	exec.stopCh = make(chan struct{})
	stopCh := exec.stopCh
	exec.submitters.New = func() interface{} {
		return &executorSubmitterImpl{
			ch: make(chan *executor, 1),
		}
	}
	go func() {
		var scratch []*executorSubmitterImpl
		for {
			exec.clean(&scratch)
			select {
			case <-stopCh:
				return
			default:
				time.Sleep(exec.maxExecutorIdleDuration)
			}
		}
	}()
}

func (exec *executors) clean(scratch *[]*executorSubmitterImpl) {
	maxExecutorIdleDuration := exec.maxExecutorIdleDuration
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
	for i := range tmp {
		tmp[i].ch <- nil
		tmp[i] = nil
	}
}

func (exec *executors) getSubmitter() *executorSubmitterImpl {
	var submitter *executorSubmitterImpl
	createExecutor := false
	exec.locker.Lock()
	ready := exec.ready
	n := len(ready) - 1
	if n < 0 {
		if exec.goroutines < exec.maxExecutorsCount {
			createExecutor = true
			exec.goroutines++
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
		submitter = vch.(*executorSubmitterImpl)
		exec.goroutinesWaitGroup.Add(1)
		go func() {
			exec.handle(submitter)
			exec.submitters.Put(vch)
		}()
	}
	exec.submittersWaitGroup.Add(1)
	return submitter
}

func (exec *executors) release(submitter *executorSubmitterImpl) bool {
	submitter.lastUseTime = time.Now()
	exec.locker.Lock()
	if exec.mustStop {
		exec.locker.Unlock()
		exec.submittersWaitGroup.Done()
		return false
	}
	exec.ready = append(exec.ready, submitter)
	exec.locker.Unlock()
	exec.submittersWaitGroup.Done()
	return true
}

func (exec *executors) handle(wch *executorSubmitterImpl) {
	for {
		e, ok := <-wch.ch
		if !ok {
			break
		}
		if e == nil {
			break
		}
		run := e.runnable
		ctx := e.ctx
		run.Run(ctx)
		if !exec.release(wch) {
			break
		}
	}
	exec.locker.Lock()
	exec.goroutines--
	exec.goroutinesWaitGroup.Done()
	exec.locker.Unlock()
}
