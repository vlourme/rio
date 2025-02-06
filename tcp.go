package rio

import (
	"context"
	"github.com/brickingsoft/errors"
	"github.com/brickingsoft/rio/pkg/aio"
	"github.com/brickingsoft/rxp/async"
	"time"
)

type TCPConnection interface {
	Connection
	Sendfile(file string) (future async.Future[int])
	MultipathTCP() bool
	SetNoDelay(noDelay bool) (err error)
	SetLinger(sec int) (err error)
	SetKeepAlive(keepalive bool) (err error)
	SetKeepAlivePeriod(period time.Duration) (err error)
	SetKeepAliveConfig(config aio.KeepAliveConfig) (err error)
}

func newTCPConnection(ctx context.Context, fd aio.NetFd) (conn TCPConnection) {
	conn = &tcpConnection{
		connection: newConnection(ctx, fd),
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
		err := errors.New(
			"sendfile failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithWrap(errors.Define("no file specified")),
		)
		future = async.FailedImmediately[int](conn.ctx, err)
		return
	}
	if conn.disconnected() {
		err := errors.New(
			"sendfile failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithWrap(ErrClosed),
		)
		future = async.FailedImmediately[int](conn.ctx, err)
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
		err := errors.New(
			"sendfile failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithWrap(promiseErr),
		)
		future = async.FailedImmediately[int](conn.ctx, err)
		return
	}

	aio.Sendfile(conn.fd, file, func(userdata aio.Userdata, err error) {
		n := userdata.N
		if err != nil {
			err = errors.New(
				"sendfile failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithWrap(err),
			)
		}
		promise.Complete(n, err)
		return
	})

	future = promise.Future()
	return
}
