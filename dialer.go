package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/aio"
	"github.com/brickingsoft/rxp"
	"github.com/brickingsoft/rxp/async"
)

func Dial(ctx context.Context, network string, address string, options ...Option) (future async.Future[Connection]) {
	opts := &Options{}
	for _, o := range options {
		err := o(opts)
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
		if async.IsBusy(promiseErr) {
			future = async.FailedImmediately[Connection](ctx, ErrBusy)
		} else {
			future = async.FailedImmediately[Connection](ctx, promiseErr)
		}
		return
	}
	future = promise.Future()

	executed := rxp.TryExecute(ctx, func() {
		connectOpts := aio.ConnectOptions{
			MultipathTCP: opts.MultipathTCP,
			LocalAddr:    opts.DialPacketConnLocalAddr,
		}
		aio.Connect(network, address, connectOpts, func(result int, userdata aio.Userdata, err error) {
			if err != nil {
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
				conn = newUnixConnection(ctx, connFd)
				break
			case "ip", "ip4", "ip6":
				conn = newIPConnection(ctx, connFd)
				break
			}

			if n := opts.DefaultConnReadTimeout; n > 0 {
				err = conn.SetReadTimeout(n)
				if err != nil {
					conn.Close().OnComplete(async.DiscardVoidHandler)
					promise.Fail(err)
					return
				}
			}
			if n := opts.DefaultConnWriteTimeout; n > 0 {
				err = conn.SetWriteTimeout(n)
				if err != nil {
					conn.Close().OnComplete(async.DiscardVoidHandler)
					promise.Fail(err)
					return
				}
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

			promise.Succeed(conn)
			return
		})
	})

	if !executed {
		promise.Fail(ErrBusy)
		return
	}

	return
}
