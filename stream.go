package rio

import (
	"context"
	"crypto/tls"
	"errors"
	"github.com/brickingsoft/rio/pkg/rate/timeslimiter"
	"github.com/brickingsoft/rio/pkg/security"
	"github.com/brickingsoft/rio/pkg/sockets"
	"github.com/brickingsoft/rxp"
	"github.com/brickingsoft/rxp/async"
	"net"
	"runtime"
	"time"
)

type Listener interface {
	Addr() (addr net.Addr)
	Accept() (future async.Future[Connection])
	Close() (err error)
}

func Listen(ctx context.Context, network string, addr string, options ...Option) (ln Listener, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	opt := Options{
		RxpOptions:                       rxp.Options{},
		ParallelAcceptors:                runtime.NumCPU() * 2,
		MaxConnections:                   DefaultMaxConnections,
		MaxConnectionsLimiterWaitTimeout: DefaultMaxConnectionsLimiterWaitTimeout,
		TLSConfig:                        nil,
		MultipathTCP:                     false,
	}
	for _, option := range options {
		err = option(&opt)
		if err != nil {
			return
		}
	}
	// executors
	executors := rxp.New(opt.AsRxpOptions()...)
	ctx = rxp.With(ctx, executors)
	// connections limiter

	connectionsLimiter := timeslimiter.New(opt.MaxConnections)
	ctx = timeslimiter.With(ctx, connectionsLimiter)
	if opt.MaxConnections > 0 && opt.MaxConnections < int64(opt.ParallelAcceptors) {
		opt.ParallelAcceptors = int(opt.MaxConnections)
	}

	// listen
	inner, innerErr := sockets.Listen(network, addr, sockets.Options{
		MultipathTCP: opt.MultipathTCP,
	})
	if innerErr != nil {
		_ = executors.Close()
		err = errors.Join(errors.New("rio: listen failed"), innerErr)
		return
	}
	ln = &listener{
		ctx:                           ctx,
		network:                       network,
		inner:                         inner,
		connectionsLimiter:            connectionsLimiter,
		connectionsLimiterWaitTimeout: opt.MaxConnectionsLimiterWaitTimeout,
		executors:                     executors,
		tlsConfig:                     opt.TLSConfig,
		promises:                      make([]async.Promise[Connection], opt.ParallelAcceptors),
	}
	return
}

type listener struct {
	ctx                           context.Context
	network                       string
	inner                         sockets.Listener
	connectionsLimiter            *timeslimiter.Bucket
	connectionsLimiterWaitTimeout time.Duration
	executors                     rxp.Executors
	tlsConfig                     *tls.Config
	promises                      []async.Promise[Connection]
}

func (ln *listener) Addr() (addr net.Addr) {
	addr = ln.inner.Addr()
	return
}

func (ln *listener) Accept() (future async.Future[Connection]) {
	ctx := ln.ctx
	promisesLen := len(ln.promises)
	futures := make([]async.Future[Connection], promisesLen)
	for i := 0; i < promisesLen; i++ {
		promise, promiseErr := async.MustStreamPromise[Connection](ctx, 8)
		if promiseErr != nil {
			future = async.FailedImmediately[Connection](ctx, promiseErr)
			_ = ln.Close()
			return
		}
		ln.acceptOne(promise, 0)
		ln.promises[i] = promise
		futures[i] = promise.Future()
	}
	future = async.JoinStreamFutures[Connection](futures)
	return
}

func (ln *listener) Close() (err error) {
	for _, promise := range ln.promises {
		promise.Cancel()
	}
	err = ln.inner.Close()
	closeExecErr := ln.executors.CloseGracefully()
	if closeExecErr != nil {
		if err == nil {
			err = closeExecErr
		} else {
			err = errors.Join(err, closeExecErr)
		}
	}
	return
}

func (ln *listener) ok() bool {
	return ln.ctx.Err() == nil
}

const (
	ms10 = 10 * time.Millisecond
)

func (ln *listener) acceptOne(streamPromise async.Promise[Connection], limitedTimes int) {
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
	ln.inner.Accept(func(sock sockets.Connection, err error) {
		if err != nil {
			streamPromise.Fail(err)
			return
		}
		if ln.tlsConfig != nil {
			sock = security.Serve(ln.ctx, sock, ln.tlsConfig).(sockets.Connection)
		}
		var conn Connection
		switch ln.network {
		case "tcp", "tcp4", "tcp6":
			conn = newTCPConnection(ln.ctx, sock)
			streamPromise.Succeed(conn)
			break
		case "unix", "unixpacket":
			conn = newUnixConnection(ln.ctx, sock)
			streamPromise.Succeed(conn)
			break
		default:
			// not matched, so close it
			_ = sock.Close()
			streamPromise.Fail(ErrNetworkDisMatched)
			break
		}
		ln.acceptOne(streamPromise, 0)
		return
	})
}
