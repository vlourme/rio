package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/async"
	"github.com/brickingsoft/rio/pkg/sockets"
	"net"
	"runtime"
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

func Listen(ctx context.Context, network string, addr string, options ...Option) (ln Listener, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	opt := Options{
		parallelAcceptors: runtime.NumCPU() * 2,
		tlsConfig:         nil,
		multipathTCP:      false,
		proto:             0,
		pollers:           0,
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
	if opt.maxExecutorIdleDuration > 0 {
		executorsOptions = append(executorsOptions, async.MaxIdleExecutorDuration(opt.maxExecutorIdleDuration))
	}
	executors := async.New(executorsOptions...)
	ctx = async.With(ctx, executors)
	// listen
	switch network {
	case "tcp", "tcp4", "tcp6":
		inner, listenTCPErr := sockets.ListenTCP(network, addr, sockets.Options{
			MultipathTCP: opt.multipathTCP,
			Proto:        opt.proto,
			Pollers:      opt.pollers,
		})
		if listenTCPErr != nil {
			err = listenTCPErr
			return
		}
		ln = &tcpListener{
			ctx:       ctx,
			inner:     inner,
			executors: executors,
			tlsConfig: opt.tlsConfig,
			promises:  make([]async.Promise[Connection], opt.parallelAcceptors),
		}
		break
	case "unix":
		inner, listenTCPErr := sockets.ListenUnix(network, addr, sockets.Options{
			Proto:   opt.proto,
			Pollers: opt.pollers,
		})
		if listenTCPErr != nil {
			err = listenTCPErr
			return
		}
		ln = &unixListener{
			ctx:       ctx,
			inner:     inner,
			executors: executors,
			tlsConfig: opt.tlsConfig,
			promises:  make([]async.Promise[Connection], opt.parallelAcceptors),
		}
		break
	default:
		err = errors.New("rio: network not supported")
		return
	}
	return
}

func IsClosed(err error) bool {
	return errors.Is(err, ErrClosed) || errors.Is(err, context.Canceled) || errors.Is(err, async.ErrFutureWasClosed)
}
