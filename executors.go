package rio

import (
	"github.com/brickingsoft/rxp"
	"runtime"
	"sync"
)

var (
	executors     rxp.Executors = nil
	executorsOnce sync.Once
)

func Startup(options ...rxp.Option) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(error); ok {
				err = e
			}
		}
	}()
	executors = rxp.New(options...)
	return err
}

func Shutdown() (err error) {
	runtime.SetFinalizer(executors, nil)
	err = Executors().CloseGracefully()
	return err
}

func Executors() rxp.Executors {
	executorsOnce.Do(func() {
		if executors == nil {
			executors = rxp.New()
			runtime.SetFinalizer(executors, rxp.Executors.CloseGracefully)
		}
	})
	return executors
}
