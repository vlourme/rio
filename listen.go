package rio

import (
	"context"
	"crypto/tls"
	"errors"
	"github.com/brickingsoft/rio/pkg/aio"
	"github.com/brickingsoft/rio/pkg/rate/timeslimiter"
	"github.com/brickingsoft/rio/security"
	"github.com/brickingsoft/rxp"
	"github.com/brickingsoft/rxp/async"
	"net"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const (
	defaultAcceptMaxConnections                   = int64(0)
	defaultAcceptMaxConnectionsLimiterWaitTimeout = 500 * time.Millisecond
)

type ListenOptions struct {
	Options
	ParallelAcceptors                      int
	AcceptMaxConnections                   int64
	AcceptMaxConnectionsLimiterWaitTimeout time.Duration
	UnixListenerUnlinkOnClose              bool
}

// WithParallelAcceptors
// 设置并行链接接受器数量。
//
// 默认值为 runtime.NumCPU() * 2。
// 注意：当值大于 ListenOptions.ParallelAcceptors，即 WithParallelAcceptors 所设置的值。
// 则并行链接接受器数为最大链接数。
func WithParallelAcceptors(parallelAcceptors int) Option {
	return func(options *Options) (err error) {
		cpuNum := runtime.NumCPU() * 2
		if parallelAcceptors < 1 || cpuNum < parallelAcceptors {
			parallelAcceptors = cpuNum
		}
		opts := (*ListenOptions)(unsafe.Pointer(options))
		opts.ParallelAcceptors = parallelAcceptors
		return
	}
}

// WithAcceptMaxConnections
// 设置最大链接数。默认为0即无上限。
func WithAcceptMaxConnections(maxConnections int64) Option {
	return func(options *Options) (err error) {
		if maxConnections > 0 {
			opts := (*ListenOptions)(unsafe.Pointer(options))
			opts.AcceptMaxConnections = maxConnections
		}
		return
	}
}

// WithAcceptMaxConnectionsLimiterWaitTimeout
// 设置最大链接数限制器等待超时。默认为500毫秒。
//
// 当10次都没新链接，当前协程会被挂起。
func WithAcceptMaxConnectionsLimiterWaitTimeout(maxConnectionsLimiterWaitTimeout time.Duration) Option {
	return func(options *Options) (err error) {
		if maxConnectionsLimiterWaitTimeout > 0 {
			opts := (*ListenOptions)(unsafe.Pointer(options))
			opts.AcceptMaxConnectionsLimiterWaitTimeout = maxConnectionsLimiterWaitTimeout
		}
		return
	}
}

// WithUnixListenerUnlinkOnClose
// 设置unix监听器是否在关闭时取消地址链接。用于链接型地址。
func WithUnixListenerUnlinkOnClose() Option {
	return func(options *Options) (err error) {
		opts := (*ListenOptions)(unsafe.Pointer(options))
		opts.UnixListenerUnlinkOnClose = true
		return
	}
}

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
	opt := ListenOptions{
		Options: Options{
			DefaultConnReadTimeout:     0,
			DefaultConnWriteTimeout:    0,
			DefaultConnReadBufferSize:  0,
			DefaultConnWriteBufferSize: 0,
			DefaultInboundBufferSize:   0,
			TLSConfig:                  nil,
			TLSConnectionBuilder:       security.Server,
			MultipathTCP:               false,
			PromiseMakeOptions:         make([]async.Option, 0, 1),
		},
		ParallelAcceptors:                      runtime.NumCPU() * 2,
		AcceptMaxConnections:                   defaultAcceptMaxConnections,
		AcceptMaxConnectionsLimiterWaitTimeout: defaultAcceptMaxConnectionsLimiterWaitTimeout,
		UnixListenerUnlinkOnClose:              false,
	}
	for _, option := range options {
		err = option((*Options)(unsafe.Pointer(&opt)))
		if err != nil {
			return
		}
	}

	// parallel acceptors
	parallelAcceptors := opt.ParallelAcceptors
	// connections limiter
	maxConnections := opt.AcceptMaxConnections
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
		err = &net.OpError{Op: aio.OpListen, Net: network, Err: listenErr}
		return
	}
	// handle unix
	unlinkOnClose := false
	if network == "unix" || network == "unixpacket" {
		if opt.UnixListenerUnlinkOnClose {
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
		connectionsLimiterWaitTimeout: opt.AcceptMaxConnectionsLimiterWaitTimeout,
		tlsConfig:                     opt.TLSConfig,
		tlsConnBuilder:                opt.TLSConnectionBuilder,
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
	tlsConnBuilder                security.ConnectionBuilder
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
			promise.Fail(aio.NewOpErr(aio.OpClose, ln.fd, err))
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
			if !ln.ok() || aio.IsUnexpectedCompletionError(err) {
				// discard errors when ln was closed
				return
			}
			if aio.IsBusyError(err) {
				// discard error when busy and try again
				ln.acceptOne()
				return
			}
			ln.acceptorPromises.Fail(aio.NewOpErr(aio.OpAccept, ln.fd, err))
			ln.acceptOne()
			return
		}
		connFd := userdata.Fd.(aio.NetFd)

		// create conn
		var conn Connection

		switch ln.network {
		case "tcp", "tcp4", "tcp6":
			// tls
			if ln.tlsConfig == nil {
				conn = newTCPConnection(ln.ctx, connFd)
			} else {
				sc, tlsErr := ln.tlsConnBuilder(ln.ctx, connFd, ln.tlsConfig)
				if tlsErr != nil {
					conn.Close().OnComplete(async.DiscardVoidHandler)
					ln.acceptorPromises.Fail(tlsErr)
					ln.acceptOne()
					return
				}
				conn = sc
			}
			break
		case "unix", "unixpacket":
			if ln.network == "unix" {
				// tls
				if ln.tlsConfig == nil {
					conn = newPacketConnection(ln.ctx, connFd)
				} else {
					sc, tlsErr := ln.tlsConnBuilder(ln.ctx, connFd, ln.tlsConfig)
					if tlsErr != nil {
						conn.Close().OnComplete(async.DiscardVoidHandler)
						ln.acceptorPromises.Fail(tlsErr)
						ln.acceptOne()
						return
					}
					conn = sc
				}
			} else {
				conn = newPacketConnection(ln.ctx, connFd)
			}
			break
		default:
			// not matched, so close it
			conn = newConnection(ln.ctx, connFd)
			conn.Close().OnComplete(async.DiscardVoidHandler)
			ln.acceptorPromises.Fail(aio.NewOpErr(aio.OpAccept, ln.fd, ErrNetworkUnmatched))
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
}

// *********************************************************************************************************************

type ListenPacketOptions struct {
	Options
	MulticastUDPInterface *net.Interface
}

// WithMulticastUDPInterface
// 设置组播UDP的网卡。
func WithMulticastUDPInterface(iface *net.Interface) Option {
	return func(options *Options) (err error) {
		opts := (*ListenPacketOptions)(unsafe.Pointer(options))
		opts.MulticastUDPInterface = iface
		return
	}
}

// ListenPacket
func ListenPacket(ctx context.Context, network string, addr string, options ...Option) (conn PacketConnection, err error) {
	opts := ListenPacketOptions{}
	for _, o := range options {
		err = o((*Options)(unsafe.Pointer(&opts)))
		if err != nil {
			return
		}
	}

	// executors
	ctx = rxp.With(ctx, getExecutors())
	// inner
	fd, listenErr := aio.Listen(network, addr, aio.ListenerOptions{
		MultipathTCP:       false,
		MulticastInterface: opts.MulticastUDPInterface,
	})

	if listenErr != nil {
		err = errors.Join(errors.New("rio: listen packet failed"), listenErr)
		return
	}

	conn = newPacketConnection(ctx, fd)
	return
}
