package rio

import (
	"context"
	"github.com/brickingsoft/errors"
	"github.com/brickingsoft/rio/pkg/aio"
	"github.com/brickingsoft/rxp"
	"github.com/brickingsoft/rxp/async"
	"net"
	"time"
	"unsafe"
)

type DialOptions struct {
	Options
	LocalAddr   net.Addr
	DialTimeout time.Duration
}

// WithDialLocalAddr
// 设置链接拨号器的本地地址
func WithDialLocalAddr(network string, addr string) Option {
	return func(options *Options) (err error) {
		opts := (*DialOptions)(unsafe.Pointer(options))
		opts.LocalAddr, _, _, err = aio.ResolveAddr(network, addr)
		return
	}
}

// WithDialTimeout
// 设置链接拨号器的超时
func WithDialTimeout(d time.Duration) Option {
	return func(options *Options) (err error) {
		if d < 1 {
			return
		}
		opts := (*DialOptions)(unsafe.Pointer(options))
		opts.DialTimeout = d
		return
	}
}

func Dial(ctx context.Context, network string, address string, options ...Option) (future async.Future[Connection]) {
	opts := DialOptions{
		Options: Options{
			DefaultConnReadTimeout:     0,
			DefaultConnWriteTimeout:    0,
			DefaultConnReadBufferSize:  0,
			DefaultConnWriteBufferSize: 0,
			DefaultInboundBufferSize:   0,
			TLSConnectionBuilder:       nil,
			MultipathTCP:               false,
			FastOpen:                   0,
			PromiseMode:                async.Normal,
		},
		LocalAddr:   nil,
		DialTimeout: 0,
	}
	for _, o := range options {
		opt := (*Options)(unsafe.Pointer(&opts))
		err := o(opt)
		if err != nil {
			future = async.FailedImmediately[Connection](ctx, err)
			return
		}
	}

	// exec
	_, exist := rxp.TryFrom(ctx)
	if !exist {
		ctx = rxp.With(ctx, getExecutors())
	}

	// promise
	promiseMakeOptions := make([]async.Option, 0, 1)
	if promiseModel := opts.PromiseMode; promiseModel != async.Normal {
		switch promiseModel {
		case async.Direct:
			promiseMakeOptions = append(promiseMakeOptions, async.WithDirectMode())
			break
		case async.Unlimited:
			promiseMakeOptions = append(promiseMakeOptions, async.WithUnlimitedMode())
			break
		default:
			break
		}
	}
	if timeout := opts.DialTimeout; timeout > 0 {
		promiseMakeOptions = append(promiseMakeOptions, async.WithTimeout(timeout))
	}
	promise, promiseErr := async.Make[Connection](ctx, promiseMakeOptions...)
	if promiseErr != nil {
		future = async.FailedImmediately[Connection](ctx, promiseErr)
		return
	}
	promise.SetErrInterceptor(func(ctx context.Context, conn Connection, err error) (future async.Future[Connection]) {
		if err != nil {
			if !IsEOF(err) {
				err = errors.New(
					"connect failed",
					errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
					errors.WithWrap(err),
				)
			}
		}
		future = async.Immediately[Connection](ctx, conn, err)
		return
	})
	future = promise.Future()

	// execute
	executed := rxp.TryExecute(ctx, func(ctx context.Context) {
		connectOpts := aio.ConnectOptions{
			MultipathTCP: opts.MultipathTCP,
			LocalAddr:    opts.LocalAddr,
			FastOpen:     opts.FastOpen,
		}
		aio.Connect(network, address, connectOpts, func(userdata aio.Userdata, err error) {
			if err != nil {
				err = errors.New(
					"connect failed",
					errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
					errors.WithWrap(err),
				)
				promise.Fail(err)
				return
			}
			connFd := userdata.Fd.(aio.NetFd)
			var conn Connection

			switch network {
			case "tcp", "tcp4", "tcp6":
				conn = newTCPConnection(ctx, connFd)
				break
			case "udp", "udp4", "udp6":
				conn = newPacketConnection(ctx, connFd)
				break
			case "unix", "unixgram", "unixpacket":
				if network == "unix" {
					conn = newTCPConnection(ctx, connFd)
				} else {
					conn = newPacketConnection(ctx, connFd)
				}
				break
			case "ip", "ip4", "ip6":
				conn = newPacketConnection(ctx, connFd)
				break
			default:
				// not matched, so close it
				aio.CloseImmediately(connFd)
				promise.Fail(ErrNetworkUnmatched)
				return
			}

			if n := opts.DefaultConnReadTimeout; n > 0 {
				conn.SetReadTimeout(n)
			}
			if n := opts.DefaultConnWriteTimeout; n > 0 {
				conn.SetWriteTimeout(n)
			}
			if n := opts.DefaultConnReadBufferSize; n > 0 {
				err = conn.SetReadBuffer(n)
				if err != nil {
					conn.Close().OnComplete(async.DiscardVoidHandler)
					promise.Fail(err)
					return
				}
			}
			if n := opts.DefaultConnWriteBufferSize; n > 0 {
				err = conn.SetWriteBuffer(n)
				if err != nil {
					conn.Close().OnComplete(async.DiscardVoidHandler)
					promise.Fail(err)
					return
				}
			}
			if n := opts.DefaultInboundBufferSize; n > 0 {
				conn.SetInboundBuffer(n)
			}

			// tls
			if opts.TLSConnectionBuilder != nil {
				conn = opts.TLSConnectionBuilder.Client(conn)
			}
			if !promise.Succeed(conn) {
				conn.Close().OnComplete(async.DiscardVoidHandler)
			}
			return
		})
	})

	if !executed {
		promise.Fail(async.Busy)
		return
	}

	return
}
