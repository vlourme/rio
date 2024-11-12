package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/async"
	"github.com/brickingsoft/rio/pkg/maxprocs"
	"github.com/brickingsoft/rio/pkg/rate/timeslimiter"
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
		minGOMAXPROCS:                    0,
		parallelAcceptors:                runtime.NumCPU() * 2,
		maxConnections:                   DefaultMaxConnections,
		maxConnectionsLimiterWaitTimeout: DefaultMaxConnectionsLimiterWaitTimeout,
		maxGoroutines:                    0,
		maxGoroutineIdleDuration:         0,
		tlsConfig:                        nil,
		multipathTCP:                     false,
	}
	for _, option := range options {
		err = option(&opt)
		if err != nil {
			return
		}
	}
	// maxprocs
	maxprocsUndo, enableMaxprocsErr := maxprocs.Enable(maxprocs.Min(opt.minGOMAXPROCS))
	if enableMaxprocsErr != nil {
		err = enableMaxprocsErr
		return
	}
	// connections limiter
	connectionsLimiter := timeslimiter.New(opt.maxConnections)
	ctx = timeslimiter.With(ctx, connectionsLimiter)
	// executors
	executorsOptions := make([]async.Option, 0, 1)
	if opt.maxGoroutines > 0 {
		executorsOptions = append(executorsOptions, async.MaxGoroutines(opt.maxGoroutines))
	}
	if opt.maxGoroutineIdleDuration > 0 {
		executorsOptions = append(executorsOptions, async.MaxGoroutineIdleDuration(opt.maxGoroutineIdleDuration))
	}
	executors := async.New(executorsOptions...)
	ctx = async.With(ctx, executors)
	// listen
	switch network {
	case "tcp", "tcp4", "tcp6":
		inner, listenTCPErr := sockets.ListenTCP(network, addr, sockets.Options{
			MultipathTCP: opt.multipathTCP,
		})
		if listenTCPErr != nil {
			err = listenTCPErr
			return
		}
		lnCtx, lnCtxCancel := context.WithCancel(ctx)
		ln = &tcpListener{
			ctx:                           lnCtx,
			cancel:                        lnCtxCancel,
			inner:                         inner,
			connectionsLimiter:            connectionsLimiter,
			connectionsLimiterWaitTimeout: opt.maxConnectionsLimiterWaitTimeout,
			executors:                     executors,
			tlsConfig:                     opt.tlsConfig,
			promises:                      make([]async.Promise[Connection], opt.parallelAcceptors),
			maxprocsUndo:                  maxprocsUndo,
		}
		break
	case "unix":
		inner, listenTCPErr := sockets.ListenUnix(network, addr, sockets.Options{})
		if listenTCPErr != nil {
			err = listenTCPErr
			return
		}
		lnCtx, lnCtxCancel := context.WithCancel(ctx)
		ln = &unixListener{
			ctx:                           lnCtx,
			cancel:                        lnCtxCancel,
			inner:                         inner,
			connectionsLimiter:            connectionsLimiter,
			connectionsLimiterWaitTimeout: opt.maxConnectionsLimiterWaitTimeout,
			executors:                     executors,
			tlsConfig:                     opt.tlsConfig,
			promises:                      make([]async.Promise[Connection], opt.parallelAcceptors),
			maxprocsUndo:                  maxprocsUndo,
		}
		break
	default:
		err = errors.New("rio: network not supported")
		return
	}
	return
}

func IsClosed(err error) bool {
	return errors.Is(err, ErrClosed) || errors.Is(err, context.Canceled)
}
