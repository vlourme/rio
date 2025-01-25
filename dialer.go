package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/aio"
	"github.com/brickingsoft/rxp"
	"github.com/brickingsoft/rxp/async"
	"net"
	"unsafe"
)

type DialOptions struct {
	Options
	LocalAddr net.Addr
}

// WithLocalAddr
// 设置包链接拨号器的本地地址
func WithLocalAddr(network string, addr string) Option {
	return func(options *Options) (err error) {
		opts := (*DialOptions)(unsafe.Pointer(options))
		opts.LocalAddr, _, _, err = aio.ResolveAddr(network, addr)
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
			PromiseMakeOptions:         nil,
		},
		LocalAddr: nil,
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

	// promise make options
	promiseMakeOptions := opts.PromiseMakeOptions
	if len(promiseMakeOptions) > 0 {
		ctx = async.WithOptions(ctx, promiseMakeOptions...)
	}

	promise, promiseErr := async.Make[Connection](ctx)
	if promiseErr != nil {
		future = async.FailedImmediately[Connection](ctx, promiseErr)
		return
	}
	future = promise.Future()

	executed := rxp.TryExecute(ctx, func() {
		connectOpts := aio.ConnectOptions{
			MultipathTCP: opts.MultipathTCP,
			LocalAddr:    opts.LocalAddr,
		}
		aio.Connect(network, address, connectOpts, func(userdata aio.Userdata, err error) {
			if err != nil {
				addr, _, _, _ := aio.ResolveAddr(network, address)
				err = &net.OpError{
					Op:     aio.OpDial,
					Net:    network,
					Source: nil,
					Addr:   addr,
					Err:    err,
				}
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
			promise.Succeed(conn)
			return
		})
	})

	if !executed {
		promise.Fail(async.Busy)
		return
	}

	return
}
