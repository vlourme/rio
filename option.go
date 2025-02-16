package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"net"
	"time"
)

type Options struct {
	Ctx                   context.Context
	VortexesOptions       []aio.Option
	KeepAlive             time.Duration
	KeepAliveConfig       net.KeepAliveConfig
	MultipathTCP          bool
	FastOpen              int
	MulticastUDPInterface *net.Interface
}

type Option func(options *Options) (err error)

func WithContext(ctx context.Context) Option {
	return func(options *Options) (err error) {
		options.Ctx = ctx
		return
	}
}

func WithKeepAlive(d time.Duration) Option {
	return func(options *Options) error {
		if d < 0 {
			d = 0
		}
		options.KeepAlive = d
		return nil
	}
}

func WithKeepAliveConfig(c net.KeepAliveConfig) Option {
	return func(options *Options) error {
		options.KeepAliveConfig = c
		return nil
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

// WithFastOpen
// 设置 FastOpen。
func WithFastOpen(n int) Option {
	return func(options *Options) (err error) {
		if n < 1 {
			return
		}
		if n > 999 {
			n = 256
		}
		options.FastOpen = n
		return
	}
}

// WithMulticastUDPInterface
// 设置组播UDP的网卡。
func WithMulticastUDPInterface(iface *net.Interface) Option {
	return func(options *Options) (err error) {
		options.MulticastUDPInterface = iface
		return
	}
}

func WithAIOOptions(options ...aio.Option) Option {
	return func(opts *Options) (err error) {
		opts.VortexesOptions = options
		return
	}
}
