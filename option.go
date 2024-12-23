package rio

import (
	"crypto/tls"
	"github.com/brickingsoft/rio/security"
	"github.com/brickingsoft/rxp/async"
	"time"
)

type Options struct {
	DefaultConnReadTimeout     time.Duration
	DefaultConnWriteTimeout    time.Duration
	DefaultConnReadBufferSize  int
	DefaultConnWriteBufferSize int
	DefaultInboundBufferSize   int
	TLSConfig                  *security.Config
	TLSConnectionBuilder       security.ConnectionBuilder
	MultipathTCP               bool
	PromiseMakeOptions         []async.Option
}

type Option func(options *Options) (err error)

// WithTLSConfig
// 设置TLS
func WithTLSConfig(config *security.Config) Option {
	return func(options *Options) (err error) {
		options.TLSConfig = config
		return
	}
}

// WithStandTLSConfig
// 设置TLS
func WithStandTLSConfig(config *tls.Config) Option {
	return func(options *Options) (err error) {
		options.TLSConfig = security.FromConfig(config)
		return
	}
}

// WithSecurityConnectionBuilder
// 设置 security.ConnectionBuilder。
func WithSecurityConnectionBuilder(builder security.ConnectionBuilder) Option {
	return func(options *Options) (err error) {
		if builder != nil {
			options.TLSConnectionBuilder = builder
		}
		return
	}
}

// WithMultipathTCP
// 设置多路TCP
func WithMultipathTCP() Option {
	return func(options *Options) (err error) {
		options.MultipathTCP = true
		return
	}
}

// WithDefaultConnReadTimeout
// 设置默认流链接读超时。
func WithDefaultConnReadTimeout(d time.Duration) Option {
	return func(options *Options) (err error) {
		if d > 0 {
			options.DefaultConnReadTimeout = d
		}
		return
	}
}

// WithDefaultConnWriteTimeout
// 设置默认流链接写超时。
func WithDefaultConnWriteTimeout(d time.Duration) Option {
	return func(options *Options) (err error) {
		if d > 0 {
			options.DefaultConnWriteTimeout = d
		}
		return
	}
}

// WithDefaultConnReadBufferSize
// 设置默认读缓冲区大小。
func WithDefaultConnReadBufferSize(n int) Option {
	return func(options *Options) (err error) {
		if n > 0 {
			options.DefaultConnReadBufferSize = n
		}
		return
	}
}

// WithDefaultConnWriteBufferSize
// 设置默认写缓冲区大小。
func WithDefaultConnWriteBufferSize(n int) Option {
	return func(options *Options) (err error) {
		if n > 0 {
			options.DefaultConnWriteBufferSize = n
		}
		return
	}
}

// WithDefaultInboundBufferSize
// 设置默认入站缓冲区大小。
func WithDefaultInboundBufferSize(n int) Option {
	return func(options *Options) (err error) {
		if n < 1 {
			return
		}
		options.DefaultInboundBufferSize = n
		return
	}
}

// WithPromiseMakeOptions
// 设置默认许诺构建选项。
func WithPromiseMakeOptions(promiseMakeOptions ...async.Option) Option {
	return func(options *Options) (err error) {
		options.PromiseMakeOptions = append(options.PromiseMakeOptions, promiseMakeOptions...)
		return
	}
}
