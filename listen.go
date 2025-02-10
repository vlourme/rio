package rio

import (
	"context"
	"github.com/brickingsoft/errors"
	"github.com/brickingsoft/rio/pkg/aio"
	"github.com/brickingsoft/rio/security"
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
	Close() (err error)
}

// Listen
// 监听流
func Listen(network string, addr string, options ...Option) (ln Listener, err error) {
	// opt
	opt := ListenOptions{
		Options: Options{
			DefaultConnReadTimeout:     0,
			DefaultConnWriteTimeout:    0,
			DefaultConnReadBufferSize:  0,
			DefaultConnWriteBufferSize: 0,
			DefaultInboundBufferSize:   0,
			MultipathTCP:               false,
			FastOpen:                   0,
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

	// ctx
	ctx := Background()

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
		acceptorPromises:     nil,
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
	acceptorPromises     []async.Promise[Connection]
}

func (ln *listener) Addr() (addr net.Addr) {
	addr = ln.fd.LocalAddr()
	return
}

func (ln *listener) OnAccept(fn func(ctx context.Context, conn Connection, err error)) {
	ctx := ln.ctx
	if ln.running.Load() {
		if ln.acceptorPromises != nil {
			err := errors.From(
				ErrAccept,
				errors.WithWrap(errors.New("cannot accept again")),
			)
			fn(ctx, nil, err)
			return
		}
		// accept
		ln.acceptorPromises = make([]async.Promise[Connection], ln.parallelAcceptors)
		for i := 0; i < ln.parallelAcceptors; i++ {
			acceptorPromise, acceptorPromiseErr := async.Make[Connection](ctx, async.WithWait(), async.WithStream())
			if acceptorPromiseErr != nil {
				for j := 0; j < i; j++ {
					acceptorPromise = ln.acceptorPromises[j]
					acceptorPromise.Future().OnComplete(func(ctx context.Context, result Connection, err error) {})
					acceptorPromise.Cancel()
				}
				err := errors.From(
					ErrAccept,
					errors.WithWrap(acceptorPromiseErr),
				)
				fn(ctx, nil, err)
				return
			}
			ln.acceptorPromises[i] = acceptorPromise
		}
		acceptorFutures := make([]async.Future[Connection], ln.parallelAcceptors)
		for i, promise := range ln.acceptorPromises {
			acceptorFutures[i] = promise.Future()
			ln.acceptOne(promise)
		}

		combined := async.Combine[Connection](ctx, acceptorFutures)
		combined.OnComplete(fn)
	} else {
		err := errors.From(
			ErrAccept,
			errors.WithWrap(errors.New("listener was closed")),
		)
		fn(ln.ctx, nil, err)
	}
}

func (ln *listener) Close() (err error) {
	if !ln.running.CompareAndSwap(true, false) {
		return
	}

	// cancel acceptor
	if promises := ln.acceptorPromises; promises != nil {
		for _, promise := range promises {
			promise.Cancel()
		}
	}

	// close
	if ln.fd.Family() == syscall.AF_UNIX && ln.unlinkOnClose {
		unixAddr, isUnix := ln.fd.LocalAddr().(*net.UnixAddr)
		if isUnix {
			if path := unixAddr.String(); path[0] != '@' {
				_ = aio.Unlink(path)
			}
		}
	}
	err = aio.Close(ln.fd)
	if err != nil {
		err = errors.From(
			ErrClose,
			errors.WithWrap(err),
		)
	}
	return
}

func (ln *listener) ok() bool {
	return ln.ctx.Err() == nil && ln.running.Load()
}

func (ln *listener) acceptOne(promise async.Promise[Connection]) {
	if !ln.ok() {
		return
	}
	aio.Accept(ln.fd, func(userdata aio.Userdata, err error) {
		if err != nil {
			if ln.ok() {
				if aio.IsUnexpectedCompletionError(err) {
					err = errors.From(
						ErrAccept,
						errors.WithWrap(err),
					)
					promise.Fail(err)
					_ = ln.Close()
				} else if aio.IsBusy(err) {
					ln.acceptOne(promise)
				} else {
					err = errors.From(
						ErrAccept,
						errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
						errors.WithWrap(err),
					)
					promise.Fail(err)
					ln.acceptOne(promise)
				}
			} else {
				_ = ln.Close()
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
			_ = aio.Close(connFd)
			err = errors.From(
				ErrAccept,
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithWrap(ErrNetworkUnmatched),
			)
			promise.Fail(err)
			ln.acceptOne(promise)
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
				_ = conn.Close()
				promise.Fail(err)
				ln.acceptOne(promise)
				return
			}
		}
		if ln.defaultWriteBuffer > 0 {
			err = conn.SetWriteBuffer(ln.defaultWriteBuffer)
			if err != nil {
				_ = conn.Close()
				promise.Fail(err)
				ln.acceptOne(promise)
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
		if succeed := promise.Succeed(conn); !succeed {
			_ = conn.Close()
			ln.acceptOne(promise)
			return
		}
		ln.acceptOne(promise)
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
func ListenPacket(network string, addr string, options ...Option) (conn PacketConnection, err error) {
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

	// ctx
	ctx := Background()
	// conn
	conn = newPacketConnection(ctx, fd)
	return
}
