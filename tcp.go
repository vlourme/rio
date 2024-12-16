package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/aio"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"time"
)

type TCPConnection interface {
	Connection
	Sendfile(file string) (future async.Future[transport.Outbound])
	MultipathTCP() bool
	SetNoDelay(noDelay bool) (err error)
	SetLinger(sec int) (err error)
	SetKeepAlive(keepalive bool) (err error)
	SetKeepAlivePeriod(period time.Duration) (err error)
	SetKeepAliveConfig(config aio.KeepAliveConfig) (err error)
}

func newTCPConnection(ctx context.Context, fd aio.NetFd) (conn TCPConnection) {
	c := newConnection(ctx, fd)
	conn = &tcpConnection{
		connection: *c,
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

func (conn *tcpConnection) Sendfile(file string) (future async.Future[transport.Outbound]) {
	if len(file) == 0 {
		future = async.FailedImmediately[transport.Outbound](conn.ctx, aio.NewOpErr(aio.OpSendfile, conn.fd, errors.New("no file specified")))
		return
	}
	promise, promiseErr := async.Make[transport.Outbound](conn.ctx)
	if promiseErr != nil {
		if async.IsBusy(promiseErr) {
			future = async.FailedImmediately[transport.Outbound](conn.ctx, aio.NewOpErr(aio.OpSendfile, conn.fd, ErrBusy))
		} else {
			future = async.FailedImmediately[transport.Outbound](conn.ctx, aio.NewOpErr(aio.OpSendfile, conn.fd, promiseErr))
		}
		return
	}
	aio.Sendfile(conn.fd, file, func(n int, userdata aio.Userdata, err error) {
		if err != nil {
			if n == 0 {
				promise.Fail(aio.NewOpErr(aio.OpSendfile, conn.fd, err))
			} else {
				outbound := transport.NewOutBound(n, aio.NewOpErr(aio.OpSendfile, conn.fd, err))
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
