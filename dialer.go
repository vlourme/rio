package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/sockets"
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
		socketOpts := sockets.Options{
			MultipathTCP:            opts.MultipathTCP,
			DialPacketConnLocalAddr: opts.DialPacketConnLocalAddr,
		}
		sockets.Dial(network, address, socketOpts, func(inner sockets.Connection, err error) {
			if err != nil {
				promise.Fail(err)
				return
			}

			var conn Connection

			switch network {
			case "tcp", "tcp4", "tcp6":
				conn = newTCPConnection(ctx, inner)
				break
			case "udp", "udp4", "udp6":
				packetInner, ok := inner.(sockets.PacketConnection)
				if !ok {
					inner.Close(func(err error) {})
					promise.Fail(errors.New("sockets.PacketConnection is not a sockets.PacketConnection"))
					break
				}
				conn = newPacketConnection(ctx, packetInner)
				break
			case "unix", "unixgram", "unixpacket":
				packetInner, ok := inner.(sockets.PacketConnection)
				if !ok {
					inner.Close(func(err error) {})
					promise.Fail(errors.New("sockets.PacketConnection is not a sockets.PacketConnection"))
					break
				}
				conn = newUnixConnection(ctx, packetInner)
				break
			case "ip", "ip4", "ip6":
				packetInner, ok := inner.(sockets.PacketConnection)
				if !ok {
					inner.Close(func(err error) {})
					promise.Fail(errors.New("sockets.PacketConnection is not a sockets.PacketConnection"))
					break
				}
				conn = newIPConnection(ctx, packetInner)
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
