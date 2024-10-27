package async

type Runnable interface {
	Run(fn func())
}

var (
	defaultRunnable Runnable = &simpleRunnable{}
)

func RegisterRunnable(runnable Runnable) {
	defaultRunnable = runnable
}

type simpleRunnable struct {
}

func (runnable *simpleRunnable) Run(fn func()) {
	go fn()
}
