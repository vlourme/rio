package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/aio"
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

type ListenOptions struct {
	Options
	ParallelAcceptors         int
	UnixListenerUnlinkOnClose bool
	FastOpen                  int
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

// WithUnixListenerUnlinkOnClose
// 设置unix监听器是否在关闭时取消地址链接。用于链接型地址。
func WithUnixListenerUnlinkOnClose() Option {
	return func(options *Options) (err error) {
		opts := (*ListenOptions)(unsafe.Pointer(options))
		opts.UnixListenerUnlinkOnClose = true
		return
	}
}

// WithFastOpen
// 设置 FastOpen。
func WithFastOpen(n int) Option {
	return func(options *Options) (err error) {
		opts := (*ListenOptions)(unsafe.Pointer(options))
		if n > 999 {
			n = 256
		}
		opts.FastOpen = n
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
			MultipathTCP:               false,
			PromiseMakeOptions:         make([]async.Option, 0, 1),
		},
		ParallelAcceptors:         runtime.NumCPU() * 2,
		UnixListenerUnlinkOnClose: false,
	}
	for _, option := range options {
		err = option((*Options)(unsafe.Pointer(&opt)))
		if err != nil {
			return
		}
	}

	// parallel acceptors
	parallelAcceptors := opt.ParallelAcceptors

	// sockets listen
	fd, listenErr := aio.Listen(network, addr, aio.ListenerOptions{
		MultipathTCP:       opt.MultipathTCP,
		MulticastInterface: nil,
		FastOpen:           opt.FastOpen,
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
	acceptorPromises, acceptorPromiseErr := async.StreamPromises[Connection](ctx, parallelAcceptors, async.WithDirectMode())
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
		ctx:                  ctx,
		running:              running,
		network:              network,
		fd:                   fd,
		unlinkOnClose:        unlinkOnClose,
		tlsConnBuilder:       opt.TLSConnectionBuilder,
		defaultReadTimeout:   opt.DefaultConnReadTimeout,
		defaultWriteTimeout:  opt.DefaultConnWriteTimeout,
		defaultReadBuffer:    opt.DefaultConnReadBufferSize,
		defaultWriteBuffer:   opt.DefaultConnWriteBufferSize,
		defaultInboundBuffer: opt.DefaultInboundBufferSize,
		parallelAcceptors:    parallelAcceptors,
		acceptorPromises:     acceptorPromises,
	}
	return
}

type listener struct {
	ctx                  context.Context
	running              *atomic.Bool
	network              string
	fd                   aio.NetFd
	unlinkOnClose        bool
	tlsConnBuilder       security.ConnectionBuilder
	defaultReadTimeout   time.Duration
	defaultWriteTimeout  time.Duration
	defaultReadBuffer    int
	defaultWriteBuffer   int
	defaultInboundBuffer int
	parallelAcceptors    int
	acceptorPromises     async.Promise[Connection]
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
	// cancel acceptor
	ln.acceptorPromises.Cancel()

	promise, promiseErr := async.Make[async.Void](ln.ctx, async.WithUnlimitedMode())
	if promiseErr != nil {
		if ln.fd.Family() == syscall.AF_UNIX && ln.unlinkOnClose {
			unixAddr, isUnix := ln.fd.LocalAddr().(*net.UnixAddr)
			if isUnix {
				if path := unixAddr.String(); path[0] != '@' {
					_ = aio.Unlink(path)
				}
			}
		}
		aio.CloseImmediately(ln.fd)
		return
	}
	if ln.fd.Family() == syscall.AF_UNIX && ln.unlinkOnClose {
		unixAddr, isUnix := ln.fd.LocalAddr().(*net.UnixAddr)
		if isUnix {
			if path := unixAddr.String(); path[0] != '@' {
				_ = aio.Unlink(path)
			}
		}
	}

	aio.Close(ln.fd, func(userdata aio.Userdata, err error) {
		if err != nil {
			aio.CloseImmediately(ln.fd)
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
	aio.Accept(ln.fd, func(userdata aio.Userdata, err error) {
		if err != nil {
			if aio.IsUnexpectedCompletionError(err) {
				// shutdown then close ln
				if ln.ok() {
					ln.running.Store(false)
					aio.CloseImmediately(ln.fd)
					ln.acceptorPromises.Cancel()
				}
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
			conn = newTCPConnection(ln.ctx, connFd)
			break
		case "unix", "unixpacket":
			conn = newTCPConnection(ln.ctx, connFd)
			break
		default:
			// not matched, so close it
			aio.CloseImmediately(connFd)
			ln.acceptorPromises.Fail(aio.NewOpErr(aio.OpAccept, ln.fd, ErrNetworkUnmatched))
			ln.acceptOne()
			return
		}
		// set default
		if ln.defaultReadTimeout > 0 {
			conn.SetReadTimeout(ln.defaultReadTimeout)
		}
		if ln.defaultWriteTimeout > 0 {
			conn.SetWriteTimeout(ln.defaultWriteTimeout)
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
		// tls
		if ln.tlsConnBuilder != nil {
			conn = ln.tlsConnBuilder.Server(conn)
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
