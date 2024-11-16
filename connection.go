package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/rate/timeslimiter"
	"github.com/brickingsoft/rio/pkg/sockets"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"net"
	"time"
)

type Connection interface {
	Context() (ctx context.Context)
	LocalAddr() (addr net.Addr)
	RemoteAddr() (addr net.Addr)
	SetDeadline(t time.Time) (err error)
	SetReadDeadline(t time.Time) (err error)
	SetWriteDeadline(t time.Time) (err error)
	SetReadBufferSize(size int)
	Read() (future async.Future[transport.Inbound])
	Write(p []byte) (future async.Future[transport.Outbound])
	Close() (err error)
}

const (
	defaultReadBufferSize = 1024
)

func newConnection(ctx context.Context, inner sockets.Connection) (conn *connection) {
	connCtx, cancel := context.WithCancel(ctx)
	conn = &connection{
		ctx:    connCtx,
		cancel: cancel,
		inner:  inner,
		rb:     transport.NewInboundBuffer(),
		rbs:    defaultReadBufferSize,
		rto:    0,
		wto:    0,
	}
	return
}

type connection struct {
	ctx    context.Context
	cancel context.CancelFunc
	inner  sockets.Connection
	rb     transport.InboundBuffer
	rbs    int
	rto    time.Duration
	wto    time.Duration
}

func (conn *connection) Context() (ctx context.Context) {
	ctx = conn.ctx
	return
}

func (conn *connection) LocalAddr() (addr net.Addr) {
	addr = conn.inner.LocalAddr()
	return
}

func (conn *connection) RemoteAddr() (addr net.Addr) {
	addr = conn.inner.RemoteAddr()
	return
}

func (conn *connection) SetDeadline(t time.Time) (err error) {
	timeout := time.Until(t)
	if timeout < 1 {
		err = errors.New("rio: deadline too short")
		return
	}
	err = conn.inner.SetDeadline(t)
	if err != nil {
		return
	}
	conn.rto = timeout
	conn.wto = timeout
	return
}

func (conn *connection) SetReadDeadline(t time.Time) (err error) {
	timeout := time.Until(t)
	if timeout < 1 {
		err = errors.New("rio: deadline too short")
		return
	}
	err = conn.inner.SetReadDeadline(t)
	if err != nil {
		return
	}
	conn.rto = timeout
	return
}

func (conn *connection) SetWriteDeadline(t time.Time) (err error) {
	timeout := time.Until(t)
	if timeout < 1 {
		err = errors.New("rio: deadline too short")
		return
	}
	err = conn.inner.SetWriteDeadline(t)
	if err != nil {
		return
	}
	conn.wto = timeout
	return
}

func (conn *connection) SetReadBufferSize(size int) {
	if size < 1 {
		size = defaultReadBufferSize
	}
	conn.rbs = size
	return
}

func (conn *connection) Read() (future async.Future[transport.Inbound]) {
	promise, ok := async.TryPromise[transport.Inbound](conn.ctx)
	if !ok {
		future = async.FailedImmediately[transport.Inbound](conn.ctx, ErrBusy)
		return
	}

	if conn.rto > 0 {
		timeout := time.Now().Add(conn.rto)
		promise.SetDeadline(timeout)
	}

	p := conn.rb.Allocate(conn.rbs)
	conn.inner.Read(p, func(n int, err error) {
		conn.rb.AllocatedWrote(n)
		if err != nil {
			promise.Fail(err)
			return
		}
		inbound := transport.NewInbound(conn.rb, n)
		promise.Succeed(inbound)
		return
	})
	future = promise.Future()
	return
}

func (conn *connection) Write(p []byte) (future async.Future[transport.Outbound]) {
	if len(p) == 0 {
		future = async.FailedImmediately[transport.Outbound](conn.ctx, ErrEmptyPacket)
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

	conn.inner.Write(p, func(n int, err error) {
		if err != nil {
			if n == 0 {
				promise.Fail(err)
			} else {
				outbound := transport.NewOutBound(n, err)
				promise.Succeed(outbound)
			}
			return
		}
		outbound := transport.NewOutBound(n, err)
		promise.Succeed(outbound)
		return
	})

	future = promise.Future()
	return
}

func (conn *connection) Close() (err error) {
	conn.cancel()
	err = conn.inner.Close()
	conn.rb.Close()
	timeslimiter.TryRevert(conn.ctx)
	return
}
