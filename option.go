package rio

import (
	"crypto/tls"
	"runtime"
	"time"
)

type Options struct {
	loops                  int // one loop one acceptor, multi-acceptors use one promise
	maxExecutors           int
	maxExecuteIdleDuration time.Duration
	tlsConfig              *tls.Config
	multipathTCP           bool
	proto                  int
	pollers                int
}

type Option func(options *Options) (err error)

func WithLoops(loops int) Option {
	return func(options *Options) (err error) {
		cpuNum := runtime.NumCPU() * 2
		if loops < 1 || cpuNum < loops {
			loops = cpuNum
		}
		options.loops = loops
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

func WithProto(proto int) Option {
	return func(options *Options) (err error) {
		options.proto = proto
		return
	}
}

func WithPollers(pollers int) Option {
	return func(options *Options) (err error) {
		options.pollers = pollers
		return
	}
}

func WithMaxExecutors(maxExecutors int) Option {
	return func(options *Options) (err error) {
		options.maxExecutors = maxExecutors
		return
	}
}

func WithMaxExecuteIdleDuration(maxExecuteIdleDuration time.Duration) Option {
	return func(options *Options) (err error) {
		options.maxExecuteIdleDuration = maxExecuteIdleDuration
		return
	}
}
