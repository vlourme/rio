package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/aio"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"net"
	"sync/atomic"
	"time"
)

type Connection interface {
	transport.Connection
}

const (
	defaultReadBufferSize = 1024
)

func newConnection(ctx context.Context, fd aio.NetFd) connection {
	conn := connection{
		ctx:    ctx,
		fd:     fd,
		closed: &atomic.Bool{},
		rb:     transport.NewInboundBuffer(),
		rbs:    defaultReadBufferSize,
	}
	return conn
}

type connection struct {
	ctx          context.Context
	fd           aio.NetFd
	closed       *atomic.Bool
	readTimeout  time.Duration
	writeTimeout time.Duration
	rb           transport.InboundBuffer
	rbs          int
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

func (conn *connection) SetReadTimeout(d time.Duration) {
	if d < 1 {
		d = 0
	}
	conn.readTimeout = d
	return
}

func (conn *connection) SetWriteTimeout(d time.Duration) {
	if d < 1 {
		d = 0
	}
	conn.writeTimeout = d
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
	if conn.disconnected() {
		future = async.FailedImmediately[transport.Inbound](conn.ctx, ErrClosed)
		return
	}
	b, allocateErr := conn.rb.Allocate(conn.rbs)
	if allocateErr != nil {
		future = async.FailedImmediately[transport.Inbound](conn.ctx, errors.Join(ErrAllocate, allocateErr))
		return
	}
	var promise async.Promise[transport.Inbound]
	var promiseErr error
	if conn.readTimeout > 0 {
		promise, promiseErr = async.Make[transport.Inbound](conn.ctx, async.WithTimeout(conn.readTimeout))
	} else {
		promise, promiseErr = async.Make[transport.Inbound](conn.ctx)
	}
	if promiseErr != nil {
		conn.rb.AllocatedWrote(0)
		future = async.FailedImmediately[transport.Inbound](conn.ctx, promiseErr)
		return
	}
	promise.SetErrInterceptor(conn.handleReadErrInterceptor)

	aio.Recv(conn.fd, b, func(userdata aio.Userdata, err error) {
		n := userdata.N
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

func (conn *connection) Write(b []byte) (future async.Future[int]) {
	if conn.disconnected() {
		future = async.FailedImmediately[int](conn.ctx, ErrClosed)
		return
	}
	bLen := len(b)
	if bLen == 0 {
		future = async.FailedImmediately[int](conn.ctx, ErrEmptyBytes)
		return
	}
	var promise async.Promise[int]
	var promiseErr error
	if conn.writeTimeout > 0 {
		promise, promiseErr = async.Make[int](conn.ctx, async.WithTimeout(conn.writeTimeout))
	} else {
		promise, promiseErr = async.Make[int](conn.ctx)
	}
	if promiseErr != nil {
		future = async.FailedImmediately[int](conn.ctx, promiseErr)
		return
	}
	promise.SetErrInterceptor(conn.handleWriteErrInterceptor)

	conn.write(promise, b, bLen, 0)
	future = promise.Future()
	return
}

func (conn *connection) write(promise async.Promise[int], b []byte, bLen int, written int) {
	aio.Send(conn.fd, b[written:], func(userdata aio.Userdata, err error) {
		if err != nil {
			err = aio.NewOpErr(aio.OpWrite, conn.fd, err)
			written += userdata.N
			promise.Complete(written, err)
			return
		}

		written += userdata.N

		if written == bLen {
			promise.Succeed(written)
			return
		}

		conn.write(promise, b, bLen, written)
		return
	})

	return
}

func (conn *connection) Close() (future async.Future[async.Void]) {
	if conn.closed.CompareAndSwap(false, true) {
		promise, promiseErr := async.Make[async.Void](conn.ctx, async.WithUnlimitedMode(), async.WithWait())
		if promiseErr != nil {
			aio.CloseImmediately(conn.fd)
			conn.rb.Close()
			future = async.SucceedImmediately[async.Void](conn.ctx, async.Void{})
			return
		}
		aio.Close(conn.fd, func(userdata aio.Userdata, err error) {
			if err != nil {
				promise.Fail(aio.NewOpErr(aio.OpClose, conn.fd, err))
			} else {
				promise.Succeed(async.Void{})
			}
			conn.rb.Close()
			return
		})
		future = promise.Future()
	} else {
		future = async.SucceedImmediately[async.Void](conn.ctx, async.Void{})
	}
	return
}

func (conn *connection) disconnected() bool {
	return conn.closed.Load()
}

func (conn *connection) handleReadErrInterceptor(ctx context.Context, inbound transport.Inbound, err error) (future async.Future[transport.Inbound]) {
	if IsEOF(err) {
		future = async.Immediately[transport.Inbound](ctx, inbound, err)
		return
	}
	if IsDeadlineExceeded(err) || IsUnexpectedContextFailed(err) {
		aio.Cancel(conn.fd.ReadOperator())
	} else if IsShutdown(err) {
		aio.CloseImmediately(conn.fd)
	}

	err = aio.NewOpErr(aio.OpRead, conn.fd, err)
	future = async.Immediately[transport.Inbound](ctx, inbound, err)
	return
}

func (conn *connection) handleWriteErrInterceptor(ctx context.Context, n int, err error) (future async.Future[int]) {
	if IsDeadlineExceeded(err) || IsUnexpectedContextFailed(err) {
		aio.Cancel(conn.fd.WriteOperator())
	} else if IsShutdown(err) {
		aio.CloseImmediately(conn.fd)
	}

	err = aio.NewOpErr(aio.OpWrite, conn.fd, err)
	future = async.Immediately[int](ctx, n, err)
	return
}
