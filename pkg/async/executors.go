package async

import "context"

type Runnable interface {
	Run(ctx context.Context)
}

type runnableFunc struct {
	ctx context.Context
	fn  func(ctx context.Context)
}

func (exec *runnableFunc) Run(ctx context.Context) {
	exec.fn(ctx)
}

func RunnableFunc(ctx context.Context, fn func(ctx context.Context)) Runnable {
	return &runnableFunc{
		ctx: ctx,
		fn:  fn,
	}
}

type ExecutorChan interface {
	Push(ctx context.Context, runnable Runnable)
}

type Executors interface {
	TryEmit(executor Runnable) (ok bool)
	Emit(executor Runnable)
	TryGetExecutorChan() (ch ExecutorChan, ok bool)
	Close() (err error)
}
