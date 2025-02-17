package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"net"
	"time"
)

type ListenOptions struct {
	KeepAlive       time.Duration
	KeepAliveConfig net.KeepAliveConfig
	MultipathTCP    bool
	FastOpen        int
	Ctx             context.Context
	UseSendZC       bool
	VortexesOptions []aio.Option
}

type ListenOption func(options *ListenOptions) (err error)

func WithContext(ctx context.Context) ListenOption {
	return func(options *ListenOptions) (err error) {
		options.Ctx = ctx
		return
	}
}

func WithKeepAlive(d time.Duration) ListenOption {
	return func(options *ListenOptions) error {
		if d < 0 {
			d = 0
		}
		options.KeepAlive = d
		return nil
	}
}

func WithKeepAliveConfig(c net.KeepAliveConfig) ListenOption {
	return func(options *ListenOptions) error {
		options.KeepAliveConfig = c
		return nil
	}
}

// WithMultipathTCP
// 设置多路TCP
func WithMultipathTCP() ListenOption {
	return func(options *ListenOptions) (err error) {
		options.MultipathTCP = true
		return
	}
}

// WithFastOpen
// 设置 FastOpen。
func WithFastOpen(n int) ListenOption {
	return func(options *ListenOptions) (err error) {
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

// WithUseSendZC
// 设置是否使用 SendZC 选项。
func WithUseSendZC(use bool) ListenOption {
	return func(options *ListenOptions) error {
		options.UseSendZC = use
		return nil
	}
}

// WithAIOOptions
// 设置 AIO 选项。
func WithAIOOptions(options ...aio.Option) ListenOption {
	return func(opts *ListenOptions) (err error) {
		opts.VortexesOptions = options
		return
	}
}

// *********************************************************************************************************************

// ListenPacket
// 监听包
//func ListenPacket(network string, addr string, options ...Option) (conn transport.PacketConnection, err error) {
//	opts := ListenPacketOptions{}
//	for _, o := range options {
//		err = o((*Options)(unsafe.Pointer(&opts)))
//		if err != nil {
//			err = errors.New(
//				"listen packet failed",
//				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
//				errors.WithWrap(err),
//			)
//			return
//		}
//	}
//
//	// inner
//	fd, listenErr := aio.Listen(network, addr, aio.ListenerOptions{
//		MultipathTCP:       false,
//		MulticastInterface: opts.MulticastUDPInterface,
//	})
//
//	if listenErr != nil {
//		err = errors.New(
//			"listen packet failed",
//			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
//			errors.WithWrap(listenErr),
//		)
//		return
//	}
//
//	// ctx
//	ctx := Background()
//	// conn
//	conn = newPacketConnection(ctx, fd)
//	return
//}
