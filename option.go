package rio

import (
	"crypto/tls"
	"github.com/brickingsoft/rxp"
	"github.com/brickingsoft/rxp/pkg/maxprocs"
	"runtime"
	"time"
)

const (
	DefaultMaxConnections                   = int64(0)
	DefaultMaxConnectionsLimiterWaitTimeout = 500 * time.Millisecond
)

type Options struct {
	ro                               rxp.Options
	parallelAcceptors                int
	maxConnections                   int64
	maxConnectionsLimiterWaitTimeout time.Duration
	tlsConfig                        *tls.Config
	multipathTCP                     bool
}

func (options *Options) AsRxpOptions() []rxp.Option {
	opts := make([]rxp.Option, 0, 1)
	if n := options.ro.MaxprocsOptions.MinGOMAXPROCS; n > 0 {
		opts = append(opts, rxp.MinGOMAXPROCS(n))
	}
	if fn := options.ro.MaxprocsOptions.Procs; fn != nil {
		opts = append(opts, rxp.Procs(fn))
	}
	if fn := options.ro.MaxprocsOptions.RoundQuotaFunc; fn != nil {
		opts = append(opts, rxp.RoundQuotaFunc(fn))
	}
	if n := options.ro.MaxGoroutines; n > 0 {
		opts = append(opts, rxp.MaxGoroutines(n))
	}
	if n := options.ro.MaxReadyGoroutinesIdleDuration; n > 0 {
		opts = append(opts, rxp.MaxReadyGoroutinesIdleDuration(n))
	}
	if n := options.ro.CloseTimeout; n > 0 {
		opts = append(opts, rxp.WithCloseTimeout(n))
	}
	return opts
}

type Option func(options *Options) (err error)

func ParallelAcceptors(parallelAcceptors int) Option {
	return func(options *Options) (err error) {
		cpuNum := runtime.NumCPU() * 2
		if parallelAcceptors < 1 || cpuNum < parallelAcceptors {
			parallelAcceptors = cpuNum
		}
		options.parallelAcceptors = parallelAcceptors
		return
	}
}

func MaxConnections(maxConnections int64) Option {
	return func(options *Options) (err error) {
		if maxConnections > 0 {
			options.maxConnections = maxConnections
		}
		return
	}
}

func MaxConnectionsLimiterWaitTimeout(maxConnectionsLimiterWaitTimeout time.Duration) Option {
	return func(options *Options) (err error) {
		if maxConnectionsLimiterWaitTimeout > 0 {
			options.maxConnectionsLimiterWaitTimeout = maxConnectionsLimiterWaitTimeout
		}
		return
	}
}

func TLSConfig(config *tls.Config) Option {
	return func(options *Options) (err error) {
		options.tlsConfig = config
		return
	}
}
func MultipathTCP() Option {
	return func(options *Options) (err error) {
		options.multipathTCP = true
		return
	}
}

// MinGOMAXPROCS
// 最小 GOMAXPROCS 值，只在 linux 环境下有效。一般用于 docker 容器环境。
func MinGOMAXPROCS(n int) Option {
	return func(options *Options) error {
		return rxp.MinGOMAXPROCS(n)(&options.ro)
	}
}

// Procs
// 设置最大 GOMAXPROCS 构建函数。
func Procs(fn maxprocs.ProcsFunc) Option {
	return func(options *Options) error {
		return rxp.Procs(fn)(&options.ro)
	}
}

// RoundQuotaFunc
// 设置整数配额函数
func RoundQuotaFunc(fn maxprocs.RoundQuotaFunc) Option {
	return func(options *Options) error {
		return rxp.RoundQuotaFunc(fn)(&options.ro)
	}
}

// MaxGoroutines
// 设置最大协程数
func MaxGoroutines(n int) Option {
	return func(options *Options) error {
		return rxp.MaxGoroutines(n)(&options.ro)
	}
}

// MaxReadyGoroutinesIdleDuration
// 设置准备中协程最大闲置时长
func MaxReadyGoroutinesIdleDuration(d time.Duration) Option {
	return func(options *Options) error {
		return rxp.MaxReadyGoroutinesIdleDuration(d)(&options.ro)
	}
}

// WithCloseTimeout
// 设置关闭超时时长
func WithCloseTimeout(timeout time.Duration) Option {
	return func(options *Options) error {
		return rxp.WithCloseTimeout(timeout)(&options.ro)
	}
}
