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
	"sync/atomic"
	"time"
)

type Listener interface {
	Addr() (addr net.Addr)
	// Accept
	// 准备接收一个链接
	//
	// 接收器是一个流，无需多次调用，当关闭时会返回一个 context.Canceled 错误。
	// 注意：当具备并行接收时，未来的 handler 是线程不安全的。
	Accept() (future async.Future[Connection])
	Close() (future async.Future[async.Void])
}

func Listen(ctx context.Context, network string, addr string, options ...Option) (ln Listener, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	// opt
	opt := Options{
		StreamListenerParallelAcceptors:                      runtime.NumCPU() * 2,
		StreamListenerAcceptMaxConnections:                   DefaultStreamListenerAcceptMaxConnections,
		StreamListenerAcceptMaxConnectionsLimiterWaitTimeout: DefaultStreamListenerAcceptMaxConnectionsLimiterWaitTimeout,
		TLSConfig:                       nil,
		MultipathTCP:                    false,
		DialPacketConnLocalAddr:         nil,
		StreamUnixListenerUnlinkOnClose: false,
		DefaultConnReadTimeout:          0,
		DefaultConnWriteTimeout:         0,
		PromiseMakeOptions:              make([]async.Option, 0, 1),
	}
	for _, option := range options {
		err = option(&opt)
		if err != nil {
			return
		}
	}

	// parallel acceptors
	parallelAcceptors := opt.StreamListenerParallelAcceptors
	// connections limiter
	maxConnections := opt.StreamListenerAcceptMaxConnections
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
		err = errors.Join(errors.New("rio: listen failed"), innerErr)
		return
	}
	// handle unix
	if network == "unix" || network == "unixpacket" {
		if opt.StreamUnixListenerUnlinkOnClose {
			unixListener, ok := inner.(sockets.UnixListener)
			if !ok {
				inner.Close(nil)
				err = errors.Join(errors.New("rio: listen failed"), errors.New("unix listener is not a unix socket"))
				return
			}
			unixListener.SetUnlinkOnClose(opt.StreamUnixListenerUnlinkOnClose)
		}
	}

	// executors
	ctx = rxp.With(ctx, getExecutors())

	// conn promise
	acceptorPromises, acceptorPromiseErr := async.DirectStreamPromises[Connection](ctx, parallelAcceptors)
	if acceptorPromiseErr != nil {
		err = errors.Join(errors.New("rio: listen failed"), acceptorPromiseErr)
		return
	}
	// running
	running := new(atomic.Bool)
	running.Store(true)

	// promise make options
	promiseMakeOptions := opt.PromiseMakeOptions
	if len(promiseMakeOptions) > 0 {
		ctx = async.WithOptions(ctx, promiseMakeOptions...)
	}

	// create
	ln = &listener{
		ctx:                           ctx,
		running:                       running,
		network:                       network,
		inner:                         inner,
		connectionsLimiter:            connectionsLimiter,
		connectionsLimiterWaitTimeout: opt.StreamListenerAcceptMaxConnectionsLimiterWaitTimeout,
		tlsConfig:                     opt.TLSConfig,
		defaultReadTimeout:            opt.DefaultConnReadTimeout,
		defaultWriteTimeout:           opt.DefaultConnWriteTimeout,
		defaultReadBuffer:             opt.DefaultConnReadBufferSize,
		defaultWriteBuffer:            opt.DefaultConnWriteBufferSize,
		defaultInboundBuffer:          opt.DefaultInboundBufferSize,
		parallelAcceptors:             parallelAcceptors,
		acceptorPromises:              acceptorPromises,
	}
	return
}

type listener struct {
	ctx                           context.Context
	running                       *atomic.Bool
	network                       string
	inner                         sockets.Listener
	connectionsLimiter            *timeslimiter.Bucket
	connectionsLimiterWaitTimeout time.Duration
	tlsConfig                     *tls.Config
	defaultReadTimeout            time.Duration
	defaultWriteTimeout           time.Duration
	defaultReadBuffer             int
	defaultWriteBuffer            int
	defaultInboundBuffer          int
	parallelAcceptors             int
	acceptorPromises              async.Promise[Connection]
}

func (ln *listener) Addr() (addr net.Addr) {
	addr = ln.inner.Addr()
	return
}

func (ln *listener) Accept() (future async.Future[Connection]) {
	for i := 0; i < ln.parallelAcceptors; i++ {
		ln.acceptOne()
	}
	future = ln.acceptorPromises.Future()
	return
}

func (ln *listener) Close() (future async.Future[async.Void]) {
	if !ln.running.Load() {
		return
	}
	ln.running.Store(false)
	promise := async.UnlimitedPromise[async.Void](ln.ctx)
	ln.inner.Close(func(err error) {
		ln.acceptorPromises.Cancel()
		if err != nil {
			promise.Fail(err)
		} else {
			promise.Succeed(async.Void{})
		}
		return
	})
	future = promise.Future()
	return
}

func (ln *listener) ok() bool {
	return ln.ctx.Err() == nil && ln.running.Load()
}

func (ln *listener) acceptOne() {
	if !ln.ok() {
		return
	}

	ctx, cancel := context.WithTimeout(ln.ctx, ln.connectionsLimiterWaitTimeout)
	waitErr := ln.connectionsLimiter.Wait(ctx)
	cancel()
	if waitErr != nil {
		// not handle waitErr
		// when ln ctx canceled, then ln is closed
		return
	}
	ln.inner.Accept(func(sock sockets.Connection, err error) {
		if err != nil {
			if !ln.ok() && sockets.IsUnexpectedCompletionError(err) {
				// discard errors when ln was closed
				return
			}
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
			break
		case "unix", "unixpacket":
			conn = newUnixConnection(ln.ctx, sock)
			break
		default:
			// not matched, so close it
			sock.Close(func(err error) {})
			ln.acceptorPromises.Fail(ErrNetworkUnmatched)
			return
		}
		// set default
		if ln.defaultReadTimeout > 0 {
			err = conn.SetReadTimeout(ln.defaultReadTimeout)
			if err != nil {
				conn.Close().OnComplete(async.DiscardVoidHandler)
				ln.acceptorPromises.Fail(err)
				return
			}
		}
		if ln.defaultWriteTimeout > 0 {
			err = conn.SetWriteTimeout(ln.defaultWriteTimeout)
			if err != nil {
				conn.Close().OnComplete(async.DiscardVoidHandler)
				ln.acceptorPromises.Fail(err)
				return
			}
		}
		if ln.defaultReadBuffer > 0 {
			err = conn.SetReadBuffer(ln.defaultReadBuffer)
			if err != nil {
				conn.Close().OnComplete(async.DiscardVoidHandler)
				ln.acceptorPromises.Fail(err)
				return
			}
		}
		if ln.defaultWriteBuffer > 0 {
			err = conn.SetWriteBuffer(ln.defaultWriteBuffer)
			if err != nil {
				conn.Close().OnComplete(async.DiscardVoidHandler)
				ln.acceptorPromises.Fail(err)
				return
			}
		}
		if ln.defaultInboundBuffer != 0 {
			conn.SetInboundBuffer(ln.defaultInboundBuffer)
		}
		ln.acceptorPromises.Succeed(conn)
		ln.acceptOne()
		return
	})
}
