package rio

import (
	"github.com/brickingsoft/rxp"
	"net"
	"time"
)

type Options struct {
	KeepAlive             time.Duration
	KeepAliveConfig       net.KeepAliveConfig
	MultipathTCP          bool
	FastOpen              int
	MulticastUDPInterface *net.Interface
	Executors             rxp.Executors
}

type Option func(options *Options) (err error)

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

func WithExecutors(r rxp.Executors) Option {
	return func(options *Options) (err error) {
		options.Executors = r
		return
	}
}
