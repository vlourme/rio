package rio

import (
	"context"
	"github.com/brickingsoft/errors"
	"github.com/brickingsoft/rio/pkg/aio"
	"github.com/brickingsoft/rio/pkg/bytebuffers"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"sync/atomic"
	"time"
)

func newTCPConnection(ctx context.Context, fd aio.NetFd) (conn transport.TCPConnection) {
	conn = &tcpConnection{
		connection: connection{
			ctx:    ctx,
			fd:     fd,
			closed: &atomic.Bool{},
			rb:     bytebuffers.Acquire(),
			rbs:    defaultReadBufferSize,
		},
	}
	return
}

type tcpConnection struct {
	connection
}

func (conn *tcpConnection) MultipathTCP() bool {
	return aio.IsUsingMultipathTCP(conn.fd)
}

func (conn *tcpConnection) SetNoDelay(noDelay bool) (err error) {
	err = aio.SetNoDelay(conn.fd, noDelay)
	return
}

func (conn *tcpConnection) SetLinger(sec int) (err error) {
	err = aio.SetLinger(conn.fd, sec)
	return
}

func (conn *tcpConnection) SetKeepAlive(keepalive bool) (err error) {
	err = aio.SetKeepAlive(conn.fd, keepalive)
	return
}

func (conn *tcpConnection) SetKeepAlivePeriod(period time.Duration) (err error) {
	err = aio.SetKeepAlivePeriod(conn.fd, period)
	return
}

func (conn *tcpConnection) SetKeepAliveConfig(config aio.KeepAliveConfig) (err error) {
	err = aio.SetKeepAliveConfig(conn.fd, config)
	return
}

func (conn *tcpConnection) Sendfile(file string) (future async.Future[int]) {
	if len(file) == 0 {
		err := errors.From(
			ErrSendfile,
			errors.WithWrap(errors.Define("no file specified")),
		)
		future = async.FailedImmediately[int](conn.ctx, err)
		return
	}
	if conn.disconnected() {
		err := errors.From(
			ErrSendfile,
			errors.WithWrap(ErrClosed),
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
			ErrSendfile,
			errors.WithWrap(promiseErr),
		)
		future = async.FailedImmediately[int](conn.ctx, err)
		return
	}
	promise.SetErrInterceptor(conn.sendfileErrInterceptor)

	aio.Sendfile(conn.fd, file, func(userdata aio.Userdata, err error) {
		n := userdata.N
		if err != nil {
			err = errors.From(
				ErrSendfile,
				errors.WithWrap(err),
			)
		}
		promise.Complete(n, err)
		return
	})

	future = promise.Future()
	return
}

func (conn *connection) sendfileErrInterceptor(ctx context.Context, n int, err error) (future async.Future[int]) {
	if !errors.Is(err, ErrSendfile) {
		err = errors.From(
			ErrSendfile,
			errors.WithWrap(err),
		)
	}
	future = async.Immediately[int](ctx, n, err)
	return
}
