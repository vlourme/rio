package rio

import (
	"context"
	"crypto/tls"
	"errors"
	"github.com/brickingsoft/rio/async"
	"github.com/brickingsoft/rio/pkg/maxprocs"
	"github.com/brickingsoft/rio/pkg/rate/timeslimiter"
	"github.com/brickingsoft/rio/pkg/security"
	"github.com/brickingsoft/rio/pkg/sockets"
	"net"
	"time"
)

type TCPConnection interface {
	Connection
	SetNoDelay(noDelay bool) (err error)
	SetLinger(sec int) (err error)
	SetKeepAlive(keepalive bool) (err error)
	SetKeepAlivePeriod(period time.Duration) (err error)
}

func newTCPConnection(ctx context.Context, inner sockets.TCPConnection) (conn TCPConnection) {
	c := newConnection(ctx, inner)
	conn = &tcpConnection{
		connection: c,
	}
	return
}

type tcpConnection struct {
	connection
}

func (conn *tcpConnection) SetNoDelay(noDelay bool) (err error) {
	tcp, isTCP := conn.connection.inner.(sockets.TCPConnection)
	if !isTCP {
		err = errors.New("rio: not a TCP connection")
		return
	}
	err = tcp.SetNoDelay(noDelay)
	return
}

func (conn *tcpConnection) SetLinger(sec int) (err error) {
	tcp, isTCP := conn.connection.inner.(sockets.TCPConnection)
	if !isTCP {
		err = errors.New("rio: not a TCP connection")
		return
	}
	err = tcp.SetLinger(sec)
	return
}

func (conn *tcpConnection) SetKeepAlive(keepalive bool) (err error) {
	tcp, isTCP := conn.connection.inner.(sockets.TCPConnection)
	if !isTCP {
		err = errors.New("rio: not a TCP connection")
		return
	}
	err = tcp.SetKeepAlive(keepalive)
	return
}

func (conn *tcpConnection) SetKeepAlivePeriod(period time.Duration) (err error) {
	tcp, isTCP := conn.connection.inner.(sockets.TCPConnection)
	if !isTCP {
		err = errors.New("rio: not a TCP connection")
		return
	}
	err = tcp.SetKeepAlivePeriod(period)
	return
}

type tcpListener struct {
	ctx                           context.Context
	cancel                        context.CancelFunc
	inner                         sockets.TCPListener
	connectionsLimiter            *timeslimiter.Bucket
	connectionsLimiterWaitTimeout time.Duration
	executors                     async.Executors
	tlsConfig                     *tls.Config
	promises                      []async.Promise[Connection]
	maxprocsUndo                  maxprocs.Undo
}

func (ln *tcpListener) Addr() (addr net.Addr) {
	addr = ln.inner.Addr()
	return
}

func (ln *tcpListener) Accept() (future async.Future[Connection]) {
	ctx := ln.ctx
	promisesLen := len(ln.promises)
	for i := 0; i < promisesLen; i++ {
		promise, promiseErr := async.MustStreamPromise[Connection](ctx, 8)
		if promiseErr != nil {
			future = async.FailedImmediately[Connection](ctx, promiseErr)
			_ = ln.Close()
			return
		}
		ln.acceptOne(promise, 0)
		ln.promises[i] = promise
	}
	future = async.Group[Connection](ln.promises)
	return
}

func (ln *tcpListener) Close() (err error) {
	for _, promise := range ln.promises {
		promise.Cancel()
	}
	ln.cancel()
	err = ln.inner.Close()
	ln.executors.CloseGracefully()
	ln.maxprocsUndo()
	return
}

func (ln *tcpListener) ok() bool {
	return ln.ctx.Err() == nil
}

const (
	ms10 = 10 * time.Millisecond
)

func (ln *tcpListener) acceptOne(streamPromise async.Promise[Connection], limitedTimes int) {
	if !ln.ok() {
		streamPromise.Fail(ErrClosed)
		return
	}
	if limitedTimes > 9 {
		time.Sleep(ms10)
		limitedTimes = 0
	}
	ctx, cancel := context.WithTimeout(ln.ctx, ln.connectionsLimiterWaitTimeout)
	waitErr := ln.connectionsLimiter.Wait(ctx)
	cancel()
	if waitErr != nil {
		limitedTimes++
		ln.acceptOne(streamPromise, limitedTimes)
		return
	}
	ln.inner.Accept(func(sock sockets.TCPConnection, err error) {
		if err != nil {
			streamPromise.Fail(err)
			return
		}
		if ln.tlsConfig != nil {
			sock = security.Serve(ln.ctx, sock, ln.tlsConfig).(sockets.TCPConnection)
		}
		conn := newTCPConnection(ln.ctx, sock)
		streamPromise.Succeed(conn)
		ln.acceptOne(streamPromise, 0)
		return
	})
}
