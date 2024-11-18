package rio

import (
	"crypto/tls"
	"github.com/brickingsoft/rio/pkg/sockets"
	"net"
	"runtime"
	"time"
)

const (
	DefaultMaxConnections                   = int64(0)
	DefaultMaxConnectionsLimiterWaitTimeout = 500 * time.Millisecond
)

type Options struct {
	ParallelAcceptors                int
	MaxConnections                   int64
	MaxConnectionsLimiterWaitTimeout time.Duration
	TLSConfig                        *tls.Config
	MultipathTCP                     bool
	DialPacketConnLocalAddr          net.Addr
	UnixListenerUnlinkOnClose        bool
	DefaultStreamReadTimeout         time.Duration
	DefaultStreamWriteTimeout        time.Duration
}

type Option func(options *Options) (err error)

// WithParallelAcceptors
// 设置并行链接接受器数量。
//
// 默认值为 runtime.NumCPU() * 2。
// 注意：当值大于 Options.MaxConnections，即 WithMaxConnections 所设置的值。
// 则并行链接接受器数为最大链接数。
func WithParallelAcceptors(parallelAcceptors int) Option {
	return func(options *Options) (err error) {
		cpuNum := runtime.NumCPU() * 2
		if parallelAcceptors < 1 || cpuNum < parallelAcceptors {
			parallelAcceptors = cpuNum
		}
		options.ParallelAcceptors = parallelAcceptors
		return
	}
}

// WithMaxConnections
// 设置最大链接数。默认为0即无上限。
func WithMaxConnections(maxConnections int64) Option {
	return func(options *Options) (err error) {
		if maxConnections > 0 {
			options.MaxConnections = maxConnections
		}
		return
	}
}

// WithMaxConnectionsLimiterWaitTimeout
// 设置最大链接数限制器等待超时。默认为500毫秒。
//
// 当10次都没新链接，当前协程会被挂起。
func WithMaxConnectionsLimiterWaitTimeout(maxConnectionsLimiterWaitTimeout time.Duration) Option {
	return func(options *Options) (err error) {
		if maxConnectionsLimiterWaitTimeout > 0 {
			options.MaxConnectionsLimiterWaitTimeout = maxConnectionsLimiterWaitTimeout
		}
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

// WithUnixListenerUnlinkOnClose
// 设置unix监听器是否在关闭时取消地址链接。用于链接型地址。
func WithUnixListenerUnlinkOnClose() Option {
	return func(options *Options) (err error) {
		options.UnixListenerUnlinkOnClose = true
		return
	}
}

// WithDefaultStreamReadTimeout
// 设置默认流链接读超时。
func WithDefaultStreamReadTimeout(d time.Duration) Option {
	return func(options *Options) (err error) {
		if d > 0 {
			options.DefaultStreamReadTimeout = d
		}
		return
	}
}

// WithDefaultStreamWriteTimeout
// 设置默认流链接写超时。
func WithDefaultStreamWriteTimeout(d time.Duration) Option {
	return func(options *Options) (err error) {
		if d > 0 {
			options.DefaultStreamWriteTimeout = d
		}
		return
	}
}
