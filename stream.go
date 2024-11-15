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
	// opt
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
	// parallel acceptors
	parallelAcceptors := opt.ParallelAcceptors
	// connections limiter
	maxConnections := opt.MaxConnections
	connectionsLimiter := timeslimiter.New(maxConnections)
	ctx = timeslimiter.With(ctx, connectionsLimiter)
	if maxConnections > 0 && maxConnections < int64(parallelAcceptors) {
		parallelAcceptors = int(maxConnections)
	}

	// sockets listen
	inner, innerErr := sockets.Listen(network, addr, sockets.Options{
		MultipathTCP: opt.MultipathTCP,
	})
	if innerErr != nil {
		_ = executors.Close()
		err = errors.Join(errors.New("rio: listen failed"), innerErr)
		return
	}

	// listen ctx
	lnCTX, lnCancel := context.WithCancel(ctx)

	// conn promise
	acceptorPromises, acceptorPromiseErr := async.StreamPromises[Connection](lnCTX, parallelAcceptors, 8)
	if acceptorPromiseErr != nil {
		_ = executors.Close()
		lnCancel()
		err = errors.Join(errors.New("rio: listen failed"), acceptorPromiseErr)
		return
	}

	// create
	ln = &listener{
		ctx:                           lnCTX,
		cancel:                        lnCancel,
		network:                       network,
		inner:                         inner,
		connectionsLimiter:            connectionsLimiter,
		connectionsLimiterWaitTimeout: opt.MaxConnectionsLimiterWaitTimeout,
		executors:                     executors,
		tlsConfig:                     opt.TLSConfig,
		parallelAcceptors:             parallelAcceptors,
		acceptorPromises:              acceptorPromises,
		promises:                      make([]async.Promise[Connection], parallelAcceptors),
	}
	return
}

type listener struct {
	ctx                           context.Context
	cancel                        context.CancelFunc
	network                       string
	inner                         sockets.Listener
	connectionsLimiter            *timeslimiter.Bucket
	connectionsLimiterWaitTimeout time.Duration
	executors                     rxp.Executors
	tlsConfig                     *tls.Config
	parallelAcceptors             int
	acceptorPromises              async.Promise[Connection]
	promises                      []async.Promise[Connection]
}

func (ln *listener) Addr() (addr net.Addr) {
	addr = ln.inner.Addr()
	return
}

func (ln *listener) Accept() (future async.Future[Connection]) {
	for i := 0; i < ln.parallelAcceptors; i++ {
		ln.acceptOne(0)
	}
	future = ln.acceptorPromises.Future()
	return
}

func (ln *listener) Close() (err error) {
	ln.acceptorPromises.Cancel()
	err = ln.inner.Close()
	closeExecErr := ln.executors.CloseGracefully()
	if closeExecErr != nil {
		if err == nil {
			err = closeExecErr
		} else {
			err = errors.Join(err, closeExecErr)
		}
	}
	ln.cancel()
	return
}

func (ln *listener) ok() bool {
	return ln.ctx.Err() == nil
}

const (
	ms10 = 10 * time.Millisecond
)

func (ln *listener) acceptOne(limitedTimes int) {
	if !ln.ok() {
		ln.acceptorPromises.Fail(ErrClosed)
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
		ln.acceptOne(limitedTimes)
		return
	}
	ln.inner.Accept(func(sock sockets.Connection, err error) {
		if err != nil {
			ln.acceptorPromises.Fail(err)
			return
		}
		if ln.tlsConfig != nil {
			sock = security.Serve(ln.ctx, sock, ln.tlsConfig).(sockets.Connection)
		}
		var conn Connection
		switch ln.network {
		case "tcp", "tcp4", "tcp6":
			conn = newTCPConnection(ln.ctx, sock)
			ln.acceptorPromises.Succeed(conn)
			break
		case "unix", "unixpacket":
			conn = newUnixConnection(ln.ctx, sock)
			ln.acceptorPromises.Succeed(conn)
			break
		default:
			// not matched, so close it
			_ = sock.Close()
			ln.acceptorPromises.Fail(ErrNetworkDisMatched)
			break
		}
		ln.acceptOne(0)
		return
	})
}
