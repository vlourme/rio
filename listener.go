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
		loops:        0,
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
		loops:     opt.loops,
		tlsConfig: opt.tlsConfig,
	}
	return
}

type listener struct {
	ctx       context.Context
	inner     sockets.Listener
	executors async.Executors
	loops     int
	tlsConfig *tls.Config
}

func (ln *listener) Addr() (addr net.Addr) {
	addr = ln.inner.Addr()
	return
}

func (ln *listener) Accept() (future async.Future[Connection]) {
	//TODO implement me
	panic("implement me")
}

func (ln *listener) Close() (err error) {
	err = ln.inner.Close()
	return
}

type acceptor struct {
	ctx   context.Context
	ln    sockets.Listener
	loops int
	ch    chan Connection
}

func (acc *acceptor) OnComplete(handler async.ResultHandler[Connection]) {
	for i := 0; i < acc.loops; i++ {
		go func(acc *acceptor, handler async.ResultHandler[Connection]) {
			conn, ok := <-acc.ch
			if !ok {
				handler(acc.ctx, nil, ErrClosed)
				return
			}
			handler(acc.ctx, conn, nil)
			acc.acceptOne()
		}(acc, handler)
	}
	return
}

func (acc *acceptor) Await() (v Connection, err error) {
	ok := false
	v, ok = <-acc.ch
	if !ok {
		err = ErrClosed
	}
	return
}

func (acc *acceptor) acceptOne() {
	acc.ln.Accept(func(conn sockets.Connection, err error) {
		// todo handle err
		acc.ch <- newConnection(acc.ctx, conn)
	})
	panic("implement me")
}

func (acc *acceptor) start() {
	panic("implement me")
}

func (acc *acceptor) stop() {
	panic("implement me")
}
