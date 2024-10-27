package concurrent

type Executor interface {
	Run()
}

type funcExecutor struct {
	fn func()
}

func (exec *funcExecutor) Run() {
	exec.fn()
}

func FuncExecutor(fn func()) Executor {
	return &funcExecutor{fn: fn}
}

type Executors interface {
	Emit(executor Executor) (ok bool)
	Close() (err error)
}
