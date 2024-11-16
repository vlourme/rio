package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/sockets"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp"
	"github.com/brickingsoft/rxp/async"
	"net"
	"time"
)

// ListenPacket
func ListenPacket(ctx context.Context, network string, addr string, options ...Option) (conn PacketConnection, err error) {
	opt := Options{}
	for _, o := range options {
		err = o(&opt)
		if err != nil {
			return
		}
	}

	// executors
	executors := opt.ExtraExecutors
	hasExtraExecutors := executors != nil
	if !hasExtraExecutors {
		executors = rxp.New(opt.AsRxpOptions()...)
	}
	ctx = rxp.With(ctx, executors)
	// inner
	inner, innerErr := sockets.ListenPacket(network, addr, sockets.Options{})
	if innerErr != nil {
		if !hasExtraExecutors {
			_ = executors.Close()
		}
		err = errors.Join(errors.New("rio: listen packet failed"), innerErr)
		return
	}

	conn = newListenedPacketConnection(ctx, inner, executors, !hasExtraExecutors)
	return
}

type PacketConnection interface {
	Connection
	ReadFrom() (future async.Future[transport.PacketInbound])
	WriteTo(p []byte, addr net.Addr) (future async.Future[transport.Outbound])
	SetReadMsgOOBBufferSize(size int)
	ReadMsg() (future async.Future[transport.PacketMsgInbound])
	WriteMsg(p []byte, oob []byte, addr net.Addr) (future async.Future[transport.PacketMsgOutbound])
}

const (
	defaultOOBBufferSize = 1024
)

func newListenedPacketConnection(ctx context.Context, inner sockets.PacketConnection, executors rxp.Executors, ownedExecutors bool) (conn PacketConnection) {
	conn = &packetConnection{
		connection:     *newConnection(ctx, inner),
		executors:      executors,
		ownedExecutors: ownedExecutors,
		inner:          inner,
		oob:            transport.NewInboundBuffer(),
		oobn:           defaultOOBBufferSize,
	}
	return
}

func newPacketConnection(ctx context.Context, inner sockets.PacketConnection) (conn PacketConnection) {
	conn = &packetConnection{
		connection:     *newConnection(ctx, inner),
		executors:      nil,
		ownedExecutors: false,
		inner:          inner,
		oob:            transport.NewInboundBuffer(),
		oobn:           defaultOOBBufferSize,
	}
	return
}

type packetConnection struct {
	connection
	executors      rxp.Executors
	ownedExecutors bool
	inner          sockets.PacketConnection
	oob            transport.InboundBuffer
	oobn           int
}

func (conn *packetConnection) ReadFrom() (future async.Future[transport.PacketInbound]) {
	promise, ok := async.TryPromise[transport.PacketInbound](conn.ctx)
	if !ok {
		future = async.FailedImmediately[transport.PacketInbound](conn.ctx, ErrBusy)
		return
	}

	if conn.rto > 0 {
		timeout := time.Now().Add(conn.rto)
		promise.SetDeadline(timeout)
	}

	p := conn.rb.Allocate(conn.rbs)
	conn.inner.ReadFrom(p, func(n int, addr net.Addr, err error) {
		conn.rb.AllocatedWrote(n)
		if err != nil {
			promise.Fail(err)
			return
		}
		inbound := transport.NewPacketInbound(conn.rb, addr, n)
		promise.Succeed(inbound)
		return
	})
	future = promise.Future()
	return
}

func (conn *packetConnection) WriteTo(p []byte, addr net.Addr) (future async.Future[transport.Outbound]) {
	if len(p) == 0 {
		future = async.FailedImmediately[transport.Outbound](conn.ctx, ErrEmptyPacket)
		return
	}
	if addr == nil {
		future = async.FailedImmediately[transport.Outbound](conn.ctx, ErrNilAddr)
		return
	}

	promise, ok := async.TryPromise[transport.Outbound](conn.ctx)
	if !ok {
		future = async.FailedImmediately[transport.Outbound](conn.ctx, ErrBusy)
		return
	}

	if conn.wto > 0 {
		timeout := time.Now().Add(conn.wto)
		promise.SetDeadline(timeout)
	}

	conn.inner.WriteTo(p, addr, func(n int, err error) {
		if err != nil {
			if n == 0 {
				promise.Fail(err)
			} else {
				outbound := transport.NewOutBound(n, err)
				promise.Succeed(outbound)
			}
			return
		}
		outbound := transport.NewOutBound(n, nil)
		promise.Succeed(outbound)
		return
	})

	future = promise.Future()
	return
}

func (conn *packetConnection) SetReadMsgOOBBufferSize(size int) {
	if size < 1 {
		return
	}
	conn.oobn = size
}

func (conn *packetConnection) ReadMsg() (future async.Future[transport.PacketMsgInbound]) {
	promise, ok := async.TryPromise[transport.PacketMsgInbound](conn.ctx)
	if !ok {
		future = async.FailedImmediately[transport.PacketMsgInbound](conn.ctx, ErrBusy)
		return
	}

	if conn.rto > 0 {
		timeout := time.Now().Add(conn.rto)
		promise.SetDeadline(timeout)
	}

	p := conn.rb.Allocate(conn.rbs)
	oob := conn.oob.Allocate(conn.oobn)
	conn.inner.ReadMsg(p, oob, func(n int, oobn int, flags int, addr net.Addr, err error) {
		conn.rb.AllocatedWrote(n)
		conn.oob.AllocatedWrote(oobn)
		if err != nil {
			promise.Fail(err)
			return
		}
		inbound := transport.NewPacketMsgInbound(conn.rb, conn.oob, addr, n, oobn, flags)
		promise.Succeed(inbound)
		return
	})
	future = promise.Future()
	return
}

func (conn *packetConnection) WriteMsg(p []byte, oob []byte, addr net.Addr) (future async.Future[transport.PacketMsgOutbound]) {
	if len(p) == 0 {
		future = async.FailedImmediately[transport.PacketMsgOutbound](conn.ctx, ErrEmptyPacket)
		return
	}
	if addr == nil {
		future = async.FailedImmediately[transport.PacketMsgOutbound](conn.ctx, ErrNilAddr)
		return
	}

	promise, ok := async.TryPromise[transport.PacketMsgOutbound](conn.ctx)
	if !ok {
		future = async.FailedImmediately[transport.PacketMsgOutbound](conn.ctx, ErrBusy)
		return
	}

	if conn.wto > 0 {
		timeout := time.Now().Add(conn.wto)
		promise.SetDeadline(timeout)
	}

	conn.inner.WriteMsg(p, oob, addr, func(n int, oobn int, err error) {
		if err != nil {
			if n == 0 {
				promise.Fail(err)
			} else {
				outbound := transport.NewPacketMsgOutbound(n, oobn, err)
				promise.Succeed(outbound)
			}
			return
		}
		outbound := transport.NewPacketMsgOutbound(n, oobn, nil)
		promise.Succeed(outbound)
		return
	})

	future = promise.Future()
	return
}

func (conn *packetConnection) Close() (err error) {
	err = conn.connection.Close()
	if conn.ownedExecutors {
		closeExecErr := conn.executors.CloseGracefully()
		if closeExecErr != nil {
			if err == nil {
				err = closeExecErr
			} else {
				err = errors.Join(err, closeExecErr)
			}
		}
	}
	return
}
