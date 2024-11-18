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

// Startup
// 启动执行器
//
// rio 是基于 rxp.Executors 异步编程模式。
// 默认提供一个执行器，如果需要定制化，则使用 Startup 完成。
// 注意：必须在程序起始位置调用，否则无效。
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

// Shutdown
// 关闭执行器
//
// 非优雅的，即不会等待所有协程执行完毕。
//
// 一般使用 ShutdownGracefully 来实现等待所有协程执行完毕。
func Shutdown() error {
	runtime.SetFinalizer(executors, nil)
	return Executors().Close()
}

// ShutdownGracefully
// 优雅的关闭执行器
//
// 它会等待所有协程执行完毕。
//
// 如果需要支持超时机制，则需要在 Startup 里进行设置。
func ShutdownGracefully() error {
	runtime.SetFinalizer(executors, nil)
	return Executors().CloseGracefully()
}

// Executors
// 获取执行器
func Executors() rxp.Executors {
	executorsOnce.Do(func() {
		if executors == nil {
			executors = rxp.New()
			runtime.SetFinalizer(executors, rxp.Executors.CloseGracefully)
		}
	})
	return executors
}
