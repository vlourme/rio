package rio

import (
	"crypto/tls"
	"github.com/brickingsoft/rio/pkg/sockets"
	"github.com/brickingsoft/rxp"
	"github.com/brickingsoft/rxp/pkg/maxprocs"
	"net"
	"runtime"
	"time"
)

const (
	DefaultMaxConnections                   = int64(0)
	DefaultMaxConnectionsLimiterWaitTimeout = 500 * time.Millisecond
)

type Options struct {
	RxpOptions                       rxp.Options
	ParallelAcceptors                int
	MaxConnections                   int64
	MaxConnectionsLimiterWaitTimeout time.Duration
	TLSConfig                        *tls.Config
	MultipathTCP                     bool
	DialPacketConnLocalAddr          net.Addr
}

func (options *Options) AsRxpOptions() []rxp.Option {
	opts := make([]rxp.Option, 0, 1)
	if n := options.RxpOptions.MaxprocsOptions.MinGOMAXPROCS; n > 0 {
		opts = append(opts, rxp.MinGOMAXPROCS(n))
	}
	if fn := options.RxpOptions.MaxprocsOptions.Procs; fn != nil {
		opts = append(opts, rxp.Procs(fn))
	}
	if fn := options.RxpOptions.MaxprocsOptions.RoundQuotaFunc; fn != nil {
		opts = append(opts, rxp.RoundQuotaFunc(fn))
	}
	if n := options.RxpOptions.MaxGoroutines; n > 0 {
		opts = append(opts, rxp.MaxGoroutines(n))
	}
	if n := options.RxpOptions.MaxReadyGoroutinesIdleDuration; n > 0 {
		opts = append(opts, rxp.MaxReadyGoroutinesIdleDuration(n))
	}
	if n := options.RxpOptions.CloseTimeout; n > 0 {
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
		options.ParallelAcceptors = parallelAcceptors
		return
	}
}

func MaxConnections(maxConnections int64) Option {
	return func(options *Options) (err error) {
		if maxConnections > 0 {
			options.MaxConnections = maxConnections
		}
		return
	}
}

func MaxConnectionsLimiterWaitTimeout(maxConnectionsLimiterWaitTimeout time.Duration) Option {
	return func(options *Options) (err error) {
		if maxConnectionsLimiterWaitTimeout > 0 {
			options.MaxConnectionsLimiterWaitTimeout = maxConnectionsLimiterWaitTimeout
		}
		return
	}
}

func TLSConfig(config *tls.Config) Option {
	return func(options *Options) (err error) {
		options.TLSConfig = config
		return
	}
}

func MultipathTCP() Option {
	return func(options *Options) (err error) {
		options.MultipathTCP = true
		return
	}
}

func WithDialPacketConnLocalAddr(network string, addr string) Option {
	return func(options *Options) (err error) {
		options.DialPacketConnLocalAddr, _, _, err = sockets.GetAddrAndFamily(network, addr)
		return
	}
}

// MinGOMAXPROCS
// 最小 GOMAXPROCS 值，只在 linux 环境下有效。一般用于 docker 容器环境。
func MinGOMAXPROCS(n int) Option {
	return func(options *Options) error {
		return rxp.MinGOMAXPROCS(n)(&options.RxpOptions)
	}
}

// Procs
// 设置最大 GOMAXPROCS 构建函数。
func Procs(fn maxprocs.ProcsFunc) Option {
	return func(options *Options) error {
		return rxp.Procs(fn)(&options.RxpOptions)
	}
}

// RoundQuotaFunc
// 设置整数配额函数
func RoundQuotaFunc(fn maxprocs.RoundQuotaFunc) Option {
	return func(options *Options) error {
		return rxp.RoundQuotaFunc(fn)(&options.RxpOptions)
	}
}

// MaxGoroutines
// 设置最大协程数
func MaxGoroutines(n int) Option {
	return func(options *Options) error {
		return rxp.MaxGoroutines(n)(&options.RxpOptions)
	}
}

// MaxReadyGoroutinesIdleDuration
// 设置准备中协程最大闲置时长
func MaxReadyGoroutinesIdleDuration(d time.Duration) Option {
	return func(options *Options) error {
		return rxp.MaxReadyGoroutinesIdleDuration(d)(&options.RxpOptions)
	}
}

// WithCloseTimeout
// 设置关闭超时时长
func WithCloseTimeout(timeout time.Duration) Option {
	return func(options *Options) error {
		return rxp.WithCloseTimeout(timeout)(&options.RxpOptions)
	}
}
