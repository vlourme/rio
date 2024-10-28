package async

import (
	"context"
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
	Execute(ctx context.Context, runnable Runnable)
	GetExecutorSubmitter() (submitter ExecutorSubmitter, has bool)
	Available() (ok bool)
	Close()
}

type executor struct {
	ctx      context.Context
	runnable Runnable
}

type executorSubmitterImpl struct {
	lastUseTime time.Time
	ch          chan executor
}

func (submitter *executorSubmitterImpl) Submit(ctx context.Context, runnable Runnable) {
	submitter.ch <- executor{
		ctx:      ctx,
		runnable: runnable,
	}
}

const (
	ns500 = 500 * time.Nanosecond
)

func New(options ...Option) Executors {
	opt := Options{
		MaxExecutors:           defaultMaxExecutors,
		MaxExecuteIdleDuration: defaultMaxExecuteIdleDuration,
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
		maxExecutorIdleDuration: opt.MaxExecuteIdleDuration,
		locker:                  sync.Mutex{},
		count:                   0,
		running:                 0,
		mustStop:                false,
		ready:                   nil,
		stopCh:                  nil,
		submitters:              sync.Pool{},
	}
	exec.start()
	return exec
}

type executors struct {
	maxExecutorsCount       int64
	maxExecutorIdleDuration time.Duration
	locker                  sync.Mutex
	count                   int64
	running                 int64
	mustStop                bool
	ready                   []*executorSubmitterImpl
	stopCh                  chan struct{}
	submitters              sync.Pool
}

func (exec *executors) TryExecute(ctx context.Context, runnable Runnable) (ok bool) {
	if runnable == nil || atomic.LoadInt64(&exec.running) == 0 {
		return false
	}
	submitter := exec.getReady()
	if submitter == nil {
		return false
	}
	select {
	case submitter.ch <- executor{
		ctx:      ctx,
		runnable: runnable,
	}:
		ok = true
		break
	case <-ctx.Done():
		break
	}
	return
}

func (exec *executors) Execute(ctx context.Context, runnable Runnable) {
	if runnable == nil || atomic.LoadInt64(&exec.running) == 0 {
		return
	}
	times := 10
	for {
		ok := exec.TryExecute(ctx, runnable)
		if ok {
			break
		}
		deadline, hasDeadline := ctx.Deadline()
		if hasDeadline && deadline.Before(time.Now()) {
			break
		}
		time.Sleep(ns500)
		times--
		if times < 0 {
			times = 10
			runtime.Gosched()
		}
	}
}

func (exec *executors) GetExecutorSubmitter() (submitter ExecutorSubmitter, has bool) {
	submitter = exec.getReady()
	has = submitter != nil
	return
}

func (exec *executors) Available() (ok bool) {
	exec.locker.Lock()
	if n := len(exec.ready) - 1; n < 0 {
		if exec.count < exec.maxExecutorsCount {
			ok = true
		}
	} else {
		ok = true
	}
	exec.locker.Unlock()
	return
}

func (exec *executors) Close() {
	atomic.StoreInt64(&exec.running, 0)
	close(exec.stopCh)
	exec.stopCh = nil
	exec.locker.Lock()
	ready := exec.ready
	for i := range ready {
		ready[i].ch <- executor{}
		ready[i] = nil
	}
	exec.ready = ready[:0]
	exec.mustStop = true
	exec.locker.Unlock()
}

func (exec *executors) start() {
	exec.running = 1
	exec.stopCh = make(chan struct{})
	stopCh := exec.stopCh
	exec.submitters.New = func() interface{} {
		return &executorSubmitterImpl{
			ch: make(chan executor, 1),
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
		tmp[i].ch <- executor{}
		tmp[i] = nil
	}
}

func (exec *executors) getReady() *executorSubmitterImpl {
	var ch *executorSubmitterImpl
	createExecutor := false
	exec.locker.Lock()
	ready := exec.ready
	n := len(ready) - 1
	if n < 0 {
		if exec.count < exec.maxExecutorsCount {
			createExecutor = true
			exec.count++
		}
	} else {
		ch = ready[n]
		ready[n] = nil
		exec.ready = ready[:n]
	}
	exec.locker.Unlock()
	if ch == nil {
		if !createExecutor {
			return nil
		}
		vch := exec.submitters.Get()
		ch = vch.(*executorSubmitterImpl)
		go func() {
			exec.handle(ch)
			exec.submitters.Put(vch)
		}()
	}
	return ch
}

func (exec *executors) release(ch *executorSubmitterImpl) bool {
	ch.lastUseTime = time.Now()
	exec.locker.Lock()
	if exec.mustStop {
		exec.locker.Unlock()
		return false
	}
	exec.ready = append(exec.ready, ch)
	exec.locker.Unlock()
	return true
}

func (exec *executors) handle(wch *executorSubmitterImpl) {
	for {
		e, ok := <-wch.ch
		if !ok {
			break
		}
		ctx := e.ctx
		run := e.runnable
		if ctx == nil || run == nil {
			break
		}
		if ctx.Err() == nil {
			run.Run(ctx)
		}
		if !exec.release(wch) {
			break
		}
	}
	exec.locker.Lock()
	exec.count--
	exec.locker.Unlock()
}
