package rio

import (
	"context"
	"crypto/tls"
	"errors"
	"github.com/brickingsoft/rio/pkg/async"
	"github.com/brickingsoft/rio/pkg/sockets"
	"net"
)

var (
	ErrClosed      = errors.New("rio: closed")
	ErrBusy        = errors.New("rio: system busy")
	ErrEmptyPacket = errors.New("rio: empty packet")
)

type Listener interface {
	Addr() (addr net.Addr)
	Accept() (future async.Future[Connection])
	Close() (err error)
}

// Listen
// ctx as root ctx, each conn can read it.
func Listen(ctx context.Context, network string, addr string, options ...Option) (ln Listener, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	opt := Options{
		loops:        1,
		tlsConfig:    nil,
		multipathTCP: false,
		proto:        0,
		pollers:      0,
	}
	for _, option := range options {
		err = option(&opt)
		if err != nil {
			return
		}
	}
	// executors
	executorsOptions := make([]async.Option, 0, 1)
	if opt.maxExecutors > 0 {
		executorsOptions = append(executorsOptions, async.MaxExecutors(opt.maxExecutors))
	}
	if opt.maxExecuteIdleDuration > 0 {
		executorsOptions = append(executorsOptions, async.MaxIdleExecuteDuration(opt.maxExecuteIdleDuration))
	}
	executors := async.New(executorsOptions...)
	ctx = async.With(ctx, executors)
	// inner
	var inner sockets.Listener
	switch network {
	case "tcp", "tcp4", "tcp6":
		inner, err = sockets.ListenTCP(network, addr, sockets.Options{
			MultipathTCP: opt.multipathTCP,
			Proto:        opt.proto,
			Pollers:      opt.pollers,
		})
		break
	case "unix":
		// todo impl listen unix
		break
	default:
		err = errors.New("rio: network not supported")
		break
	}
	if err != nil {
		return
	}
	ln = &listener{
		ctx:       ctx,
		inner:     inner,
		executors: executors,
		tlsConfig: opt.tlsConfig,
		promises:  make([]async.Promise[Connection], opt.loops),
	}
	return
}

type listener struct {
	ctx       context.Context
	inner     sockets.Listener
	executors async.Executors
	tlsConfig *tls.Config
	promises  []async.Promise[Connection]
}

func (ln *listener) Addr() (addr net.Addr) {
	addr = ln.inner.Addr()
	return
}

func (ln *listener) Accept() (future async.Future[Connection]) {
	ctx := ln.ctx
	promisesLen := len(ln.promises)
	for i := 0; i < promisesLen; i++ {
		promise, promiseErr := async.MustInfinitePromise[Connection](ctx)
		if promiseErr != nil {
			future = async.FailedImmediately[Connection](ctx, promiseErr)
			_ = ln.Close()
			return
		}
		ln.acceptOne(promise)
		ln.promises[i] = promise
	}
	future = async.Group[Connection](ln.promises)
	return
}

func (ln *listener) Close() (err error) {
	for _, promise := range ln.promises {
		promise.Cancel()
	}
	err = ln.inner.Close()
	return
}

func (ln *listener) acceptOne(infinitePromise async.Promise[Connection]) {
	ln.inner.Accept(func(sock sockets.Connection, err error) {
		if err != nil {
			infinitePromise.Fail(err)
			return
		}
		conn := newConnection(ln.ctx, sock)
		infinitePromise.Complete(conn, err)
		ln.acceptOne(infinitePromise)
		return
	})
}
