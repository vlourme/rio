package rio

import (
	"context"
	"crypto/tls"
	"errors"
	"github.com/brickingsoft/rio/pkg/aio"
	"github.com/brickingsoft/rio/pkg/rate/timeslimiter"
	"github.com/brickingsoft/rio/pkg/sockets"
	"github.com/brickingsoft/rxp"
	"github.com/brickingsoft/rxp/async"
	"net"
	"runtime"
	"sync/atomic"
	"syscall"
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
	fd, listenErr := aio.Listen(network, addr, aio.ListenerOptions{
		MultipathTCP:       opt.MultipathTCP,
		MulticastInterface: nil,
	})
	if listenErr != nil {
		err = errors.Join(errors.New("rio: listen failed"), listenErr)
		return
	}
	// handle unix
	unlinkOnClose := false
	if network == "unix" || network == "unixpacket" {
		if opt.StreamUnixListenerUnlinkOnClose {
			unlinkOnClose = true
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
		fd:                            fd,
		unlinkOnClose:                 unlinkOnClose,
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
	fd                            aio.NetFd
	unlinkOnClose                 bool
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
	addr = ln.fd.LocalAddr()
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
	if ln.fd.Family() == syscall.AF_UNIX && ln.unlinkOnClose {
		unixAddr, isUnix := ln.fd.LocalAddr().(*net.UnixAddr)
		if isUnix {
			if path := unixAddr.String(); path[0] != '@' {
				_ = aio.Unlink(path)
			}
		}
	}
	aio.Close(ln.fd, func(result int, userdata aio.Userdata, err error) {
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
	limiterCtx, limiterCtxCancel := context.WithTimeout(ln.ctx, ln.connectionsLimiterWaitTimeout)
	waitErr := ln.connectionsLimiter.Wait(limiterCtx)
	limiterCtxCancel()
	if waitErr != nil {
		if ln.ok() {
			// just wait timeout
			runtime.Gosched()
		}
		ln.acceptOne()
		return
	}

	aio.Accept(ln.fd, func(result int, userdata aio.Userdata, err error) {
		if err != nil {
			ln.connectionsLimiter.Revert()
			if !ln.ok() || sockets.IsUnexpectedCompletionError(err) {
				// discard errors when ln was closed
				return
			}
			ln.acceptorPromises.Fail(err)
			ln.acceptOne()
			return
		}
		connFd := userdata.Fd.(aio.NetFd)
		// handle tls
		if ln.tlsConfig != nil {
			//sock = security.Serve(ln.ctx, sock, ln.tlsConfig).(sockets.Connection)
		}

		// create conn
		var conn Connection

		switch ln.network {
		case "tcp", "tcp4", "tcp6":
			conn = newTCPConnection(ln.ctx, connFd)
			break
		case "unix", "unixpacket":
			conn = newUnixConnection(ln.ctx, connFd)
			break
		default:
			// not matched, so close it
			conn = newConnection(ln.ctx, connFd)
			conn.Close().OnComplete(async.DiscardVoidHandler)
			ln.acceptorPromises.Fail(ErrNetworkUnmatched)
			ln.acceptOne()
			return
		}
		// set default
		if ln.defaultReadTimeout > 0 {
			err = conn.SetReadTimeout(ln.defaultReadTimeout)
			if err != nil {
				conn.Close().OnComplete(async.DiscardVoidHandler)
				ln.acceptorPromises.Fail(err)
				ln.acceptOne()
				return
			}
		}
		if ln.defaultWriteTimeout > 0 {
			err = conn.SetWriteTimeout(ln.defaultWriteTimeout)
			if err != nil {
				conn.Close().OnComplete(async.DiscardVoidHandler)
				ln.acceptorPromises.Fail(err)
				ln.acceptOne()
				return
			}
		}
		if ln.defaultReadBuffer > 0 {
			err = conn.SetReadBuffer(ln.defaultReadBuffer)
			if err != nil {
				conn.Close().OnComplete(async.DiscardVoidHandler)
				ln.acceptorPromises.Fail(err)
				ln.acceptOne()
				return
			}
		}
		if ln.defaultWriteBuffer > 0 {
			err = conn.SetWriteBuffer(ln.defaultWriteBuffer)
			if err != nil {
				conn.Close().OnComplete(async.DiscardVoidHandler)
				ln.acceptorPromises.Fail(err)
				ln.acceptOne()
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
	//ln.inner.Accept(func(sock sockets.Connection, err error) {
	//	if err != nil {
	//		ln.connectionsLimiter.Revert()
	//		if !ln.ok() || sockets.IsUnexpectedCompletionError(err) {
	//			// discard errors when ln was closed
	//			return
	//		}
	//		ln.acceptorPromises.Fail(err)
	//		ln.acceptOne()
	//		return
	//	}
	//	// handle tls
	//	if ln.tlsConfig != nil {
	//		sock = security.Serve(ln.ctx, sock, ln.tlsConfig).(sockets.Connection)
	//	}
	//
	//	// create conn
	//	var conn Connection
	//
	//	switch ln.network {
	//	case "tcp", "tcp4", "tcp6":
	//		conn = newTCPConnection(ln.ctx, sock)
	//		break
	//	case "unix", "unixpacket":
	//		conn = newUnixConnection(ln.ctx, sock)
	//		break
	//	default:
	//		// not matched, so close it
	//		conn = newConnection(ln.ctx, sock)
	//		conn.Close().OnComplete(async.DiscardVoidHandler)
	//		ln.acceptorPromises.Fail(ErrNetworkUnmatched)
	//		ln.acceptOne()
	//		return
	//	}
	//	// set default
	//	if ln.defaultReadTimeout > 0 {
	//		err = conn.SetReadTimeout(ln.defaultReadTimeout)
	//		if err != nil {
	//			conn.Close().OnComplete(async.DiscardVoidHandler)
	//			ln.acceptorPromises.Fail(err)
	//			ln.acceptOne()
	//			return
	//		}
	//	}
	//	if ln.defaultWriteTimeout > 0 {
	//		err = conn.SetWriteTimeout(ln.defaultWriteTimeout)
	//		if err != nil {
	//			conn.Close().OnComplete(async.DiscardVoidHandler)
	//			ln.acceptorPromises.Fail(err)
	//			ln.acceptOne()
	//			return
	//		}
	//	}
	//	if ln.defaultReadBuffer > 0 {
	//		err = conn.SetReadBuffer(ln.defaultReadBuffer)
	//		if err != nil {
	//			conn.Close().OnComplete(async.DiscardVoidHandler)
	//			ln.acceptorPromises.Fail(err)
	//			ln.acceptOne()
	//			return
	//		}
	//	}
	//	if ln.defaultWriteBuffer > 0 {
	//		err = conn.SetWriteBuffer(ln.defaultWriteBuffer)
	//		if err != nil {
	//			conn.Close().OnComplete(async.DiscardVoidHandler)
	//			ln.acceptorPromises.Fail(err)
	//			ln.acceptOne()
	//			return
	//		}
	//	}
	//	if ln.defaultInboundBuffer != 0 {
	//		conn.SetInboundBuffer(ln.defaultInboundBuffer)
	//	}
	//	ln.acceptorPromises.Succeed(conn)
	//	ln.acceptOne()
	//	return
	//})
}
