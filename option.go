package rio

import (
	"crypto/tls"
	"runtime"
	"time"
)

const (
	DefaultMaxConnections                   = int64(0)
	DefaultMaxConnectionsLimiterWaitTimeout = 500 * time.Millisecond
)

type Options struct {
	minGOMAXPROCS                    int
	parallelAcceptors                int
	maxConnections                   int64
	maxConnectionsLimiterWaitTimeout time.Duration
	maxExecutors                     int
	maxExecutorIdleDuration          time.Duration
	tlsConfig                        *tls.Config
	multipathTCP                     bool
}

type Option func(options *Options) (err error)

func WithMinGOMAXPROCS(min int) Option {
	return func(options *Options) (err error) {
		if min > 2 {
			options.minGOMAXPROCS = min
		}
		return
	}
}

func WithParallelAcceptors(parallelAcceptors int) Option {
	return func(options *Options) (err error) {
		cpuNum := runtime.NumCPU() * 2
		if parallelAcceptors < 1 || cpuNum < parallelAcceptors {
			parallelAcceptors = cpuNum
		}
		options.parallelAcceptors = parallelAcceptors
		return
	}
}

func WithMaxConnections(maxConnections int64) Option {
	return func(options *Options) (err error) {
		if maxConnections > 0 {
			options.maxConnections = maxConnections
		}
		return
	}
}

func WithMaxConnectionsLimiterWaitTimeout(maxConnectionsLimiterWaitTimeout time.Duration) Option {
	return func(options *Options) (err error) {
		if maxConnectionsLimiterWaitTimeout > 0 {
			options.maxConnectionsLimiterWaitTimeout = maxConnectionsLimiterWaitTimeout
		}
		return
	}
}

func WithTLSConfig(config *tls.Config) Option {
	return func(options *Options) (err error) {
		options.tlsConfig = config
		return
	}
}
func WithMultipathTCP() Option {
	return func(options *Options) (err error) {
		options.multipathTCP = true
		return
	}
}

func WithMaxExecutors(maxExecutors int) Option {
	return func(options *Options) (err error) {
		options.maxExecutors = maxExecutors
		return
	}
}

func WithMaxExecutorIdleDuration(duration time.Duration) Option {
	return func(options *Options) (err error) {
		options.maxExecutorIdleDuration = duration
		return
	}
}
