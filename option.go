package rio

import (
	"crypto/tls"
	"github.com/brickingsoft/rio/pkg/sockets"
	"github.com/brickingsoft/rxp/async"
	"net"
	"runtime"
	"time"
)

const (
	DefaultStreamListenerAcceptMaxConnections                   = int64(0)
	DefaultStreamListenerAcceptMaxConnectionsLimiterWaitTimeout = 500 * time.Millisecond
)

type Options struct {
	StreamListenerParallelAcceptors                      int
	StreamListenerAcceptMaxConnections                   int64
	StreamListenerAcceptMaxConnectionsLimiterWaitTimeout time.Duration
	StreamUnixListenerUnlinkOnClose                      bool
	DefaultConnReadTimeout                               time.Duration
	DefaultConnWriteTimeout                              time.Duration
	DefaultConnReadBufferSize                            int
	DefaultConnWriteBufferSize                           int
	DefaultInboundBufferSize                             int
	TLSConfig                                            *tls.Config
	MultipathTCP                                         bool
	DialPacketConnLocalAddr                              net.Addr
	PromiseMakeOptions                                   []async.Option
}

type Option func(options *Options) (err error)

// WithStreamListenerParallelAcceptors
// 设置并行链接接受器数量。
//
// 默认值为 runtime.NumCPU() * 2。
// 注意：当值大于 Options.StreamListenerAcceptMaxConnections，即 WithStreamListenerAcceptMaxConnections 所设置的值。
// 则并行链接接受器数为最大链接数。
func WithStreamListenerParallelAcceptors(parallelAcceptors int) Option {
	return func(options *Options) (err error) {
		cpuNum := runtime.NumCPU() * 2
		if parallelAcceptors < 1 || cpuNum < parallelAcceptors {
			parallelAcceptors = cpuNum
		}
		options.StreamListenerParallelAcceptors = parallelAcceptors
		return
	}
}

// WithStreamListenerAcceptMaxConnections
// 设置最大链接数。默认为0即无上限。
func WithStreamListenerAcceptMaxConnections(maxConnections int64) Option {
	return func(options *Options) (err error) {
		if maxConnections > 0 {
			options.StreamListenerAcceptMaxConnections = maxConnections
		}
		return
	}
}

// WithStreamListenerAcceptMaxConnectionsLimiterWaitTimeout
// 设置最大链接数限制器等待超时。默认为500毫秒。
//
// 当10次都没新链接，当前协程会被挂起。
func WithStreamListenerAcceptMaxConnectionsLimiterWaitTimeout(maxConnectionsLimiterWaitTimeout time.Duration) Option {
	return func(options *Options) (err error) {
		if maxConnectionsLimiterWaitTimeout > 0 {
			options.StreamListenerAcceptMaxConnectionsLimiterWaitTimeout = maxConnectionsLimiterWaitTimeout
		}
		return
	}
}

// WithStreamUnixListenerUnlinkOnClose
// 设置unix监听器是否在关闭时取消地址链接。用于链接型地址。
func WithStreamUnixListenerUnlinkOnClose() Option {
	return func(options *Options) (err error) {
		options.StreamUnixListenerUnlinkOnClose = true
		return
	}
}

// WithTLSConfig
// 设置TLS
func WithTLSConfig(config *tls.Config) Option {
	return func(options *Options) (err error) {
		options.TLSConfig = config
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

// WithDialPacketConnLocalAddr
// 设置包链接拨号器的本地地址
func WithDialPacketConnLocalAddr(network string, addr string) Option {
	return func(options *Options) (err error) {
		options.DialPacketConnLocalAddr, _, _, err = sockets.GetAddrAndFamily(network, addr)
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
