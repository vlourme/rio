package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/aio"
	"github.com/brickingsoft/rio/pkg/rate/timeslimiter"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"net"
	"time"
)

type Connection interface {
	Context() (ctx context.Context)
	ConfigContext(config func(ctx context.Context) context.Context)
	LocalAddr() (addr net.Addr)
	RemoteAddr() (addr net.Addr)
	SetReadTimeout(d time.Duration) (err error)
	SetWriteTimeout(d time.Duration) (err error)
	SetReadBuffer(n int) (err error)
	SetWriteBuffer(n int) (err error)
	SetInboundBuffer(n int)
	Read() (future async.Future[transport.Inbound])
	Write(p []byte) (future async.Future[transport.Outbound])
	Close() (future async.Future[async.Void])
}

const (
	defaultReadBufferSize = 1024
)

func newConnection(ctx context.Context, fd aio.NetFd) (conn *connection) {
	conn = &connection{
		ctx: ctx,
		fd:  fd,
		rb:  transport.NewInboundBuffer(),
		rbs: defaultReadBufferSize,
	}
	return
}

type connection struct {
	ctx context.Context
	fd  aio.NetFd
	rb  transport.InboundBuffer
	rbs int
}

func (conn *connection) Context() (ctx context.Context) {
	ctx = conn.ctx
	return
}

func (conn *connection) ConfigContext(config func(ctx context.Context) context.Context) {
	if config == nil {
		return
	}
	newCtx := config(conn.ctx)
	if newCtx == nil {
		return
	}
	conn.ctx = newCtx
	return
}

func (conn *connection) LocalAddr() (addr net.Addr) {
	addr = conn.fd.LocalAddr()
	return
}

func (conn *connection) RemoteAddr() (addr net.Addr) {
	addr = conn.fd.RemoteAddr()
	return
}

func (conn *connection) SetReadTimeout(d time.Duration) (err error) {
	conn.fd.SetReadTimeout(d)
	return
}

func (conn *connection) SetWriteTimeout(d time.Duration) (err error) {
	conn.fd.SetWriteTimeout(d)
	return
}

func (conn *connection) SetReadBuffer(n int) (err error) {
	//err = conn.fd.SetReadBuffer(n)
	return
}

func (conn *connection) SetWriteBuffer(n int) (err error) {
	//err = conn.fd.SetWriteBuffer(n)
	return
}

func (conn *connection) SetInboundBuffer(size int) {
	if size < 1 {
		size = defaultReadBufferSize
	}
	conn.rbs = size
	return
}

func (conn *connection) Read() (future async.Future[transport.Inbound]) {
	p, allocateErr := conn.rb.Allocate(conn.rbs)
	if allocateErr != nil {
		future = async.FailedImmediately[transport.Inbound](conn.ctx, errors.Join(ErrAllocate, allocateErr))
		return
	}
	promise, promiseErr := async.Make[transport.Inbound](conn.ctx)
	if promiseErr != nil {
		_ = conn.rb.AllocatedWrote(0)
		if async.IsBusy(promiseErr) {
			future = async.FailedImmediately[transport.Inbound](conn.ctx, ErrBusy)
		} else {
			future = async.FailedImmediately[transport.Inbound](conn.ctx, promiseErr)
		}
		return
	}

	aio.Recv(conn.fd, p, func(n int, userdata aio.Userdata, err error) {
		if err != nil {
			_ = conn.rb.AllocatedWrote(0)
			promise.Fail(err)
			return
		}
		if awErr := conn.rb.AllocatedWrote(n); awErr != nil {
			promise.Fail(errors.Join(ErrAllocateWrote, awErr))
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
	pLen := len(p)
	if pLen == 0 {
		future = async.FailedImmediately[transport.Outbound](conn.ctx, ErrEmptyPacket)
		return
	}
	future = conn.write(p, pLen, 0)
	return
}

func (conn *connection) write(p []byte, pLen int, wrote int) (future async.Future[transport.Outbound]) {
	promise, promiseErr := async.Make[transport.Outbound](conn.ctx)
	if promiseErr != nil {
		if async.IsBusy(promiseErr) {
			future = async.FailedImmediately[transport.Outbound](conn.ctx, ErrBusy)
		} else {
			future = async.FailedImmediately[transport.Outbound](conn.ctx, promiseErr)
		}
		return
	}

	aio.Send(conn.fd, p[wrote:], func(n int, userdata aio.Userdata, err error) {
		if err != nil {
			if n == 0 {
				promise.Fail(err)
			} else {
				outbound := transport.NewOutBound(wrote+n, err)
				promise.Succeed(outbound)
			}
			return
		}

		nn := wrote + n

		if nn == pLen {
			outbound := transport.NewOutBound(nn, nil)
			promise.Succeed(outbound)
			return
		}
		conn.write(p, pLen, nn).OnComplete(func(ctx context.Context, entry transport.Outbound, cause error) {
			if cause != nil {
				if nn > 0 {
					outbound := transport.NewOutBound(nn, cause)
					promise.Succeed(outbound)
					return
				}
				promise.Fail(cause)
				return
			}
			promise.Succeed(entry)
		})
		return
	})

	future = promise.Future()
	return
}

func (conn *connection) Close() (future async.Future[async.Void]) {
	promise := async.UnlimitedPromise[async.Void](conn.ctx)
	aio.Close(conn.fd, func(result int, userdata aio.Userdata, err error) {
		if err != nil {
			promise.Fail(err)
		} else {
			promise.Succeed(async.Void{})
		}
		conn.rb.Close()
		timeslimiter.TryRevert(conn.ctx)
		return
	})
	future = promise.Future()
	return
}
