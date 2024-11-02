package rio

import (
	"crypto/tls"
	"runtime"
)

type Options struct {
	loops     int // one loop one acceptor, multi-acceptors use one promise
	tlsConfig *tls.Config
}

type Option func(options *Options) (err error)

func WithLoops(loops int) Option {
	return func(options *Options) (err error) {
		if loops < 1 {
			loops = runtime.NumCPU() * 2
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
