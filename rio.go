package rio

import (
	"errors"
	"fmt"
	"github.com/brickingsoft/rio/pkg/aio"
	"github.com/brickingsoft/rio/pkg/process"
	"github.com/brickingsoft/rio/pkg/sockets"
	"github.com/brickingsoft/rxp"
	"github.com/brickingsoft/rxp/pkg/maxprocs"
	"runtime"
	"time"
)

// Startup
// 启动
//
// rio 是基于 rxp.Executors 与 aio.Engine 的异步编程模式。
// 提供默认值，如果需要定制化，则使用 Startup 完成。
//
// 注意：必须在程序起始位置调用，否则无效。
func Startup(options ...StartupOption) (err error) {
	opts := &StartupOptions{}
	for _, option := range options {
		err = option(opts)
		if err != nil {
			err = errors.Join(errors.New("rio: startup failed"), err)
			return
		}
	}
	// process
	if opts.ProcessPriorityLevel != process.NORM {
		err = process.SetCurrentProcessPriority(opts.ProcessPriorityLevel)
		if err != nil {
			err = errors.Join(errors.New("rio: startup failed"), err)
			return
		}
	}
	// executors
	defer func() {
		if r := recover(); r != nil {
			switch e := r.(type) {
			case error:
				err = e
				if err != nil {
					err = errors.Join(errors.New("rio: startup failed"), err)
					return
				}
				break
			case string:
				err = errors.New(e)
				if err != nil {
					err = errors.Join(errors.New("rio: startup failed"), err)
					return
				}
				break
			default:
				err = errors.New(fmt.Sprintf("%+v", r))
				if err != nil {
					err = errors.Join(errors.New("rio: startup failed"), err)
					return
				}
				break
			}
		}
	}()
	executors = rxp.New(opts.ExecutorsOptions...)

	// sockets.completions
	aio.Startup(opts.AioOptions)
	return
}

// Shutdown
// 关闭
//
// 非优雅的，即不会等待所有协程执行完毕。
//
// 一般使用 ShutdownGracefully 来实现等待所有协程执行完毕。
func Shutdown() error {
	runtime.SetFinalizer(executors, nil)
	err := getExecutors().Close()
	sockets.Shutdown()
	return err
}

// ShutdownGracefully
// 优雅的关闭执行器
//
// 它会等待所有协程执行完毕。
//
// 如果需要支持超时机制，则需要在 Startup 里进行设置。
func ShutdownGracefully() error {
	runtime.SetFinalizer(executors, nil)
	err := getExecutors().CloseGracefully()
	sockets.Shutdown()
	return err
}

type StartupOptions struct {
	ProcessPriorityLevel process.PriorityLevel
	AioOptions           aio.Options
	ExecutorsOptions     []rxp.Option
}

type StartupOption func(*StartupOptions) error

// WithProcessRealtimePriorityClass
// 设置进程优先级为实时，该程序需要对应的权限。
func WithProcessRealtimePriorityClass() StartupOption {
	return func(o *StartupOptions) error {
		o.ProcessPriorityLevel = process.REALTIME
		return nil
	}
}

// WithProcessHighPriorityClass
// 设置进程优先级为高，该程序需要对应的权限。
func WithProcessHighPriorityClass() StartupOption {
	return func(o *StartupOptions) error {
		o.ProcessPriorityLevel = process.HIGH
		return nil
	}
}

// WithProcessIdlePriorityClass
// 设置进程优先级为闲置，该程序需要对应的权限。
func WithProcessIdlePriorityClass() StartupOption {
	return func(o *StartupOptions) error {
		o.ProcessPriorityLevel = process.IDLE
		return nil
	}
}

// WithMinGOMAXPROCS
// 最小 GOMAXPROCS 值，只在 linux 环境下有效。一般用于 docker 容器环境。
func WithMinGOMAXPROCS(n int) StartupOption {
	return func(o *StartupOptions) error {
		o.ExecutorsOptions = append(o.ExecutorsOptions, rxp.WithMinGOMAXPROCS(n))
		return nil
	}
}

// WithProcs
// 设置最大 GOMAXPROCS 构建函数。
func WithProcs(fn maxprocs.ProcsFunc) StartupOption {
	return func(o *StartupOptions) error {
		o.ExecutorsOptions = append(o.ExecutorsOptions, rxp.WithProcs(fn))
		return nil
	}
}

// WithRoundQuotaFunc
// 设置整数配额函数
func WithRoundQuotaFunc(fn maxprocs.RoundQuotaFunc) StartupOption {
	return func(o *StartupOptions) error {
		o.ExecutorsOptions = append(o.ExecutorsOptions, rxp.WithRoundQuotaFunc(fn))
		return nil
	}
}

// WithMaxGoroutines
// 设置最大协程数
func WithMaxGoroutines(n int) StartupOption {
	return func(o *StartupOptions) error {
		o.ExecutorsOptions = append(o.ExecutorsOptions, rxp.WithMaxGoroutines(n))
		return nil
	}
}

// WithMaxReadyGoroutinesIdleDuration
// 设置准备中协程最大闲置时长
func WithMaxReadyGoroutinesIdleDuration(d time.Duration) StartupOption {
	return func(o *StartupOptions) error {
		o.ExecutorsOptions = append(o.ExecutorsOptions, rxp.WithMaxReadyGoroutinesIdleDuration(d))
		return nil
	}
}

// WithCloseTimeout
// 设置关闭超时时长
func WithCloseTimeout(d time.Duration) StartupOption {
	return func(o *StartupOptions) error {
		o.ExecutorsOptions = append(o.ExecutorsOptions, rxp.WithCloseTimeout(d))
		return nil
	}
}

func WithAIOEngineCylinders(n int) StartupOption {
	return func(o *StartupOptions) error {
		if n > 1 {
			o.AioOptions.EngineCylinders = n
		}
		return nil
	}
}

func WithAIOFlags(n uint32) StartupOption {
	return func(o *StartupOptions) error {
		if n > 1 {
			o.AioOptions.Settings.Flags = n
		}
		return nil
	}
}

func WithAIOThreads(n int) StartupOption {
	return func(o *StartupOptions) error {
		if n > 1 {
			o.AioOptions.Settings.Threads = uint32(n)
		}
		return nil
	}
}

func WithAIOThreadIdle(n int) StartupOption {
	return func(o *StartupOptions) error {
		if n > 1 {
			o.AioOptions.Settings.ThreadIdle = uint32(n)
		}
		return nil
	}
}
