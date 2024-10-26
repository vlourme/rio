package concurrent

import "context"

type Executor interface {
	Execute(ctx context.Context)
}

type Executors interface {
	Emit(executor Executor) (ok bool)
	Close() (err error)
}
