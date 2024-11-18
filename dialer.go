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

	_, exist := rxp.TryFrom(ctx)
	if !exist {
		ctx = rxp.With(ctx, getExecutors())
	}

	promise, promiseOk := async.TryPromise[Connection](ctx)
	if !promiseOk {
		future = async.FailedImmediately[Connection](ctx, ErrBusy)
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
			switch network {
			case "tcp", "tcp4", "tcp6":
				promise.Succeed(newTCPConnection(ctx, inner))
				break
			case "udp", "udp4", "udp6":
				packetInner, ok := inner.(sockets.PacketConnection)
				if !ok {
					promise.Fail(errors.New("sockets.PacketConnection is not a sockets.PacketConnection"))
					break
				}
				promise.Succeed(newPacketConnection(ctx, packetInner))
				break
			case "unix", "unixgram", "unixpacket":
				packetInner, ok := inner.(sockets.PacketConnection)
				if !ok {
					promise.Fail(errors.New("sockets.PacketConnection is not a sockets.PacketConnection"))
					break
				}
				promise.Succeed(newUnixConnection(ctx, packetInner))
				break
			case "ip", "ip4", "ip6":
				packetInner, ok := inner.(sockets.PacketConnection)
				if !ok {
					promise.Fail(errors.New("sockets.PacketConnection is not a sockets.PacketConnection"))
					break
				}
				promise.Succeed(newIPConnection(ctx, packetInner))
				break
			}
			return
		})
	})

	if !executed {
		promise.Fail(ErrBusy)
		return
	}

	return
}
