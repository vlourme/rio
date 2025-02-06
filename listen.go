package rio

import (
	"context"
	"github.com/brickingsoft/errors"
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

// Listener
// 监听器
type Listener interface {
	// Addr
	// 地址
	Addr() (addr net.Addr)
	// OnAccept
	// 准备接收一个链接。
	// 当服务关闭时，得到一个 async.Canceled 错误。可以使用 async.IsCanceled 进行判断。
	// 当得到错误时，务必不要退出 OnAccept，请以是否收到 async.Canceled 来决定退出。
	// 不支持多次调用。
	OnAccept(fn func(ctx context.Context, conn Connection, err error))
	// Close
	// 关闭
	Close() (future async.Future[async.Void])
}

// Listen
// 监听流
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
			err = errors.New(
				"listen failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithWrap(err),
			)
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
		err = errors.New(
			"listen failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithWrap(listenErr),
		)
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
		err = errors.New(
			"listen failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithWrap(acceptorPromiseErr),
		)
		return
	}

	// promise make options
	promiseMakeOptions := opt.PromiseMakeOptions
	if len(promiseMakeOptions) > 0 {
		ctx = async.WithOptions(ctx, promiseMakeOptions...)
	}

	// running
	running := new(atomic.Bool)
	running.Store(true)

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

func (ln *listener) OnAccept(fn func(ctx context.Context, conn Connection, err error)) {
	if ln.running.Load() {
		future := ln.acceptorPromises.Future()
		future.OnComplete(fn)
		for i := 0; i < ln.parallelAcceptors; i++ {
			ln.acceptOne()
		}
	} else {
		err := errors.New(
			"accept failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithWrap(errors.New("listener was closed")),
		)
		fn(ln.ctx, nil, err)
	}
}

func (ln *listener) Close() (future async.Future[async.Void]) {
	ctx := ln.ctx

	if !ln.running.CompareAndSwap(true, false) {
		err := errors.New(
			"close failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithWrap(errors.New("listener was closed")),
		)
		future = async.FailedImmediately[async.Void](ctx, err)
		return
	}

	// cancel acceptor
	ln.acceptorPromises.Cancel()

	// close fd
	promise, promiseErr := async.Make[async.Void](ctx, async.WithUnlimitedMode())
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
		future = async.SucceedImmediately[async.Void](ctx, async.Void{})
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
			err = errors.New(
				"close failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithWrap(err),
			)
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
	aio.Accept(ln.fd, func(userdata aio.Userdata, err error) {
		if err != nil {
			if ln.ok() {
				if aio.IsUnexpectedCompletionError(err) {
					err = errors.New(
						"accept failed",
						errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
						errors.WithWrap(err),
					)
					ln.acceptorPromises.Fail(err)
					ln.Close()
				} else if aio.IsBusy(err) {
					ln.acceptOne()
				} else {
					err = errors.New(
						"accept failed",
						errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
						errors.WithWrap(err),
					)
					ln.acceptorPromises.Fail(err)
					ln.acceptOne()
				}
			} else {
				ln.Close()
			}
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
			err = errors.New(
				"accept failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithWrap(ErrNetworkUnmatched),
			)
			ln.acceptorPromises.Fail(err)
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
		// send
		if succeed := ln.acceptorPromises.Succeed(conn); !succeed {
			conn.Close().OnComplete(async.DiscardVoidHandler)
			ln.acceptOne()
			return
		}
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
// 监听包
func ListenPacket(ctx context.Context, network string, addr string, options ...Option) (conn PacketConnection, err error) {
	opts := ListenPacketOptions{}
	for _, o := range options {
		err = o((*Options)(unsafe.Pointer(&opts)))
		if err != nil {
			err = errors.New(
				"listen packet failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithWrap(err),
			)
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
		err = errors.New(
			"listen packet failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithWrap(listenErr),
		)
		return
	}

	conn = newPacketConnection(ctx, fd)
	return
}
