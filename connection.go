package rio

import (
	"context"
	"github.com/brickingsoft/errors"
	"github.com/brickingsoft/rio/pkg/aio"
	"github.com/brickingsoft/rio/pkg/bytebuffers"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"net"
	"sync/atomic"
	"time"
)

var (
	ErrRead  = errors.Define("read failed")
	ErrWrite = errors.Define("write failed")
)

type Connection interface {
	transport.Connection
}

const (
	defaultReadBufferSize = 1024
)

type connection struct {
	ctx          context.Context
	fd           aio.NetFd
	closed       *atomic.Bool
	readTimeout  time.Duration
	writeTimeout time.Duration
	rb           bytebuffers.Buffer
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
		err = errors.New(
			"set read buffer failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithWrap(err),
		)
		return
	}
	return
}

func (conn *connection) SetWriteBuffer(n int) (err error) {
	if err = aio.SetWriteBuffer(conn.fd, n); err != nil {
		err = errors.New(
			"set write buffer failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithWrap(err),
		)
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
	ctx := conn.ctx
	rb := conn.rb
	rbs := conn.rbs
	if conn.disconnected() {
		err := errors.From(
			ErrRead,
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithWrap(ErrClosed),
		)
		future = async.FailedImmediately[transport.Inbound](ctx, err)
		return
	}
	b, allocateErr := rb.Allocate(rbs)
	if allocateErr != nil {
		err := errors.From(
			ErrRead,
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithWrap(ErrAllocate),
		)
		future = async.FailedImmediately[transport.Inbound](ctx, err)
		return
	}
	var promise async.Promise[transport.Inbound]
	var promiseErr error
	if timeout := conn.readTimeout; timeout > 0 {
		promise, promiseErr = async.Make[transport.Inbound](ctx, async.WithTimeout(timeout))
	} else {
		promise, promiseErr = async.Make[transport.Inbound](ctx)
	}
	if promiseErr != nil {
		rb.Allocated(0)
		err := errors.From(
			ErrRead,
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithWrap(promiseErr),
		)
		future = async.FailedImmediately[transport.Inbound](ctx, err)
		return
	}
	promise.SetErrInterceptor(conn.readErrInterceptor)

	aio.Recv(conn.fd, b, func(userdata aio.Userdata, err error) {
		n := userdata.N
		rb.Allocated(n)
		if err != nil {
			err = errors.From(
				ErrRead,
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithWrap(err),
			)
			promise.Fail(err)
			return
		}
		inbound := rb
		promise.Succeed(inbound)
		return
	})

	future = promise.Future()
	return
}

func (conn *connection) readErrInterceptor(ctx context.Context, _ transport.Inbound, err error) (future async.Future[transport.Inbound]) {
	if !errors.Is(err, ErrRead) {
		err = errors.From(
			ErrRead,
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithWrap(err),
		)
	}
	future = async.Immediately[transport.Inbound](ctx, nil, err)
	return
}

func (conn *connection) Write(b []byte) (future async.Future[int]) {
	if conn.disconnected() {
		err := errors.From(
			ErrWrite,
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithWrap(ErrClosed),
		)
		future = async.FailedImmediately[int](conn.ctx, err)
		return
	}
	bLen := len(b)
	if bLen == 0 {
		err := errors.From(
			ErrWrite,
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithWrap(ErrEmptyBytes),
		)
		future = async.FailedImmediately[int](conn.ctx, err)
		return
	}
	var promise async.Promise[int]
	var promiseErr error
	if timeout := conn.writeTimeout; timeout > 0 {
		promise, promiseErr = async.Make[int](conn.ctx, async.WithTimeout(timeout))
	} else {
		promise, promiseErr = async.Make[int](conn.ctx)
	}
	if promiseErr != nil {
		err := errors.From(
			ErrWrite,
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithWrap(promiseErr),
		)
		future = async.FailedImmediately[int](conn.ctx, err)
		return
	}
	promise.SetErrInterceptor(conn.writeErrInterceptor)

	aio.Send(conn.fd, b, func(userdata aio.Userdata, err error) {
		n := userdata.N
		if err != nil {
			err = errors.From(
				ErrWrite,
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithWrap(err),
			)
			promise.Complete(n, err)
			return
		}
		promise.Succeed(n)
		return
	})

	future = promise.Future()
	return
}

func (conn *connection) writeErrInterceptor(ctx context.Context, n int, err error) (future async.Future[int]) {
	if !errors.Is(err, ErrWrite) {
		err = errors.From(
			ErrWrite,
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithWrap(err),
		)
	}
	future = async.Immediately[int](ctx, n, err)
	return
}

func (conn *connection) Close() (err error) {
	if conn.closed.CompareAndSwap(false, true) {
		rb := conn.rb
		conn.rb = nil
		bytebuffers.Release(rb)
		if err = aio.Close(conn.fd); err != nil {
			err = errors.New(
				"close failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithWrap(err),
			)
			return
		}
		return
	}
	return
}

func (conn *connection) disconnected() bool {
	return conn.closed.Load()
}
