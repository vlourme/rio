package async

import "context"

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
	TryExecute(executor Runnable) (ok bool)
	Execute(executor Runnable)
	GetExecutorSubmitter() (submitter ExecutorSubmitter, has bool)
	Available() (ok bool)
	Close() (err error)
}
