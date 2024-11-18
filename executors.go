package rio

import (
	"errors"
	"fmt"
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
			switch e := r.(type) {
			case error:
				err = e
				break
			case string:
				err = errors.New(e)
				break
			default:
				err = errors.New(fmt.Sprintf("%+v", r))
				break
			}
		}
	}()
	executors = rxp.New(options...)
	return
}

func Shutdown() error {
	runtime.SetFinalizer(executors, nil)
	return Executors().Close()
}

func ShutdownGracefully() error {
	runtime.SetFinalizer(executors, nil)
	return Executors().CloseGracefully()
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
