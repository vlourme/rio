package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/aio"
	"github.com/brickingsoft/rio/pkg/rate/timeslimiter"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"io"
	"net"
	"time"
)

type Connection interface {
	Context() (ctx context.Context)
	ConfigContext(config func(ctx context.Context) context.Context)
	Fd() int
	LocalAddr() (addr net.Addr)
	RemoteAddr() (addr net.Addr)
	SetReadTimeout(d time.Duration) (err error)
	SetWriteTimeout(d time.Duration) (err error)
	SetReadBuffer(n int) (err error)
	SetWriteBuffer(n int) (err error)
	SetInboundBuffer(n int)
	Read() (future async.Future[transport.Inbound])
	Write(b []byte) (future async.Future[transport.Outbound])
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

func (conn *connection) Fd() int {
	return conn.fd.Fd()
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
	if err = aio.SetReadBuffer(conn.fd, n); err != nil {
		err = aio.NewOpErr(aio.OpSet, conn.fd, err)
		return
	}
	return
}

func (conn *connection) SetWriteBuffer(n int) (err error) {
	if err = aio.SetWriteBuffer(conn.fd, n); err != nil {
		err = aio.NewOpErr(aio.OpSet, conn.fd, err)
		return
	}
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
	b, allocateErr := conn.rb.Allocate(conn.rbs)
	if allocateErr != nil {
		future = async.FailedImmediately[transport.Inbound](conn.ctx, aio.NewOpErr(aio.OpRead, conn.fd, errors.Join(ErrAllocate, allocateErr)))
		return
	}

	timeout := conn.fd.ReadTimeout()
	options := []async.Option{async.WithWaitTimeout(timeout)}
	promise, promiseErr := async.Make[transport.Inbound](conn.ctx, options...)
	if promiseErr != nil {
		_ = conn.rb.AllocatedWrote(0)
		if async.IsDeadlineExceeded(promiseErr) {
			future = async.FailedImmediately[transport.Inbound](conn.ctx, aio.NewOpErr(aio.OpRead, conn.fd, ErrDeadlineExceeded))
		} else if async.IsExecutorsClosed(promiseErr) {
			future = async.FailedImmediately[transport.Inbound](conn.ctx, aio.NewOpErr(aio.OpRead, conn.fd, ErrClosed))
			aio.CloseImmediately(conn.fd)
		} else {
			future = async.FailedImmediately[transport.Inbound](conn.ctx, aio.NewOpErr(aio.OpRead, conn.fd, promiseErr))
		}
		return
	}

	aio.Recv(conn.fd, b, func(n int, userdata aio.Userdata, err error) {
		if err != nil {
			_ = conn.rb.AllocatedWrote(0)
			if errors.Is(err, io.EOF) {
				promise.Fail(err)
			} else {
				promise.Fail(aio.NewOpErr(aio.OpRead, conn.fd, err))
			}
			return
		}
		if awErr := conn.rb.AllocatedWrote(n); awErr != nil {
			promise.Fail(aio.NewOpErr(aio.OpRead, conn.fd, errors.Join(ErrAllocate, errors.Join(ErrAllocateWrote, awErr))))
			return
		}
		inbound := transport.NewInbound(conn.rb, n)
		promise.Succeed(inbound)
		return
	})

	future = promise.Future()
	return
}

func (conn *connection) Write(b []byte) (future async.Future[transport.Outbound]) {
	bLen := len(b)
	if bLen == 0 {
		future = async.FailedImmediately[transport.Outbound](conn.ctx, aio.NewOpErr(aio.OpWrite, conn.fd, ErrEmptyBytes))
		return
	}
	future = conn.write(b, bLen, 0)
	return
}

func (conn *connection) write(b []byte, bLen int, wrote int) (future async.Future[transport.Outbound]) {
	timeout := conn.fd.WriteTimeout()
	options := []async.Option{async.WithWaitTimeout(timeout)}
	promise, promiseErr := async.Make[transport.Outbound](conn.ctx, options...)
	if promiseErr != nil {
		if async.IsDeadlineExceeded(promiseErr) {
			future = async.FailedImmediately[transport.Outbound](conn.ctx, aio.NewOpErr(aio.OpWrite, conn.fd, ErrDeadlineExceeded))
		} else if async.IsExecutorsClosed(promiseErr) {
			future = async.FailedImmediately[transport.Outbound](conn.ctx, aio.NewOpErr(aio.OpWrite, conn.fd, ErrClosed))
			aio.CloseImmediately(conn.fd)
		} else {
			future = async.FailedImmediately[transport.Outbound](conn.ctx, aio.NewOpErr(aio.OpWrite, conn.fd, promiseErr))
		}
		return
	}

	aio.Send(conn.fd, b[wrote:], func(n int, userdata aio.Userdata, err error) {
		if err != nil {
			if n == 0 {
				promise.Fail(aio.NewOpErr(aio.OpWrite, conn.fd, err))
			} else {
				outbound := transport.NewOutBound(wrote+n, aio.NewOpErr(aio.OpWrite, conn.fd, err))
				promise.Succeed(outbound)
			}
			return
		}

		nn := wrote + n

		if nn == bLen {
			outbound := transport.NewOutBound(nn, nil)
			promise.Succeed(outbound)
			return
		}
		conn.write(b, bLen, nn).OnComplete(func(ctx context.Context, entry transport.Outbound, cause error) {
			if cause != nil {
				if nn > 0 {
					outbound := transport.NewOutBound(nn, aio.NewOpErr(aio.OpWrite, conn.fd, cause))
					promise.Succeed(outbound)
					return
				}
				promise.Fail(aio.NewOpErr(aio.OpWrite, conn.fd, cause))
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
	promise, promiseErr := async.Make[async.Void](conn.ctx, async.WithUnlimitedMode(), async.WithWait())
	if promiseErr != nil {
		aio.CloseImmediately(conn.fd)
		conn.rb.Close()
		timeslimiter.TryRevert(conn.ctx)
		future = async.FailedImmediately[async.Void](conn.ctx, aio.NewOpErr(aio.OpClose, conn.fd, promiseErr))
		return
	}
	aio.Close(conn.fd, func(result int, userdata aio.Userdata, err error) {
		if err != nil {
			promise.Fail(aio.NewOpErr(aio.OpClose, conn.fd, err))
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
