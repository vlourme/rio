package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/ring"
	"net"
)

var (
	DefaultDialer = &Dialer{}
)

type Dialer struct {
	ring *ring.Ring
}

func (d *Dialer) Dial(ctx context.Context, network string, address string, options Options) (net.Conn, error) {
	// todo timeout and keep alive
	return nil, nil
}

func Dial(network string, address string, options ...Option) (net.Conn, error) {
	opts := Options{}
	for _, option := range options {
		if err := option(&opts); err != nil {
			return nil, err
		}
	}
	ctx := context.Background()
	return DefaultDialer.Dial(ctx, network, address, opts)
}

func DialContext(ctx context.Context, network string, address string, options ...Option) (net.Conn, error) {
	opts := Options{}
	for _, option := range options {
		if err := option(&opts); err != nil {
			return nil, err
		}
	}
	return DefaultDialer.Dial(ctx, network, address, opts)
}

//type DialOptions struct {
//	Options
//	LocalAddr   net.Addr
//	DialTimeout time.Duration
//}
//
//// WithDialLocalAddr
//// 设置链接拨号器的本地地址
//func WithDialLocalAddr(network string, addr string) Option {
//	return func(options *Options) (err error) {
//		opts := (*DialOptions)(unsafe.Pointer(options))
//		opts.LocalAddr, _, _, err = aio.ResolveAddr(network, addr)
//		return
//	}
//}
//
//// WithDialTimeout
//// 设置链接拨号器的超时
//func WithDialTimeout(d time.Duration) Option {
//	return func(options *Options) (err error) {
//		if d < 1 {
//			return
//		}
//		opts := (*DialOptions)(unsafe.Pointer(options))
//		opts.DialTimeout = d
//		return
//	}
//}
//
//func Dial(ctx context.Context, network string, address string, options ...Option) (future async.Future[transport.Connection]) {
//	opts := DialOptions{
//		Options: Options{
//			DefaultConnReadTimeout:     0,
//			DefaultConnWriteTimeout:    0,
//			DefaultConnReadBufferSize:  0,
//			DefaultConnWriteBufferSize: 0,
//			DefaultInboundBufferSize:   0,
//			TLSConnectionBuilder:       nil,
//			MultipathTCP:               false,
//			FastOpen:                   0,
//		},
//		LocalAddr:   nil,
//		DialTimeout: 0,
//	}
//	for _, o := range options {
//		opt := (*Options)(unsafe.Pointer(&opts))
//		err := o(opt)
//		if err != nil {
//			future = async.FailedImmediately[transport.Connection](ctx, err)
//			return
//		}
//	}
//
//	// exec
//	_, exist := rxp.TryFrom(ctx)
//	if !exist {
//		ctx = rxp.With(ctx, getExecutors())
//	}
//
//	// promise
//	promiseMakeOptions := make([]async.Option, 0, 1)
//	if timeout := opts.DialTimeout; timeout > 0 {
//		promiseMakeOptions = append(promiseMakeOptions, async.WithTimeout(timeout))
//	}
//	promise, promiseErr := async.Make[transport.Connection](ctx, promiseMakeOptions...)
//	if promiseErr != nil {
//		future = async.FailedImmediately[transport.Connection](ctx, promiseErr)
//		return
//	}
//	promise.SetErrInterceptor(dialErrInterceptor)
//	future = promise.Future()
//
//	// execute
//	task := dialer{
//		network: network,
//		address: address,
//		options: opts,
//		promise: promise,
//	}
//	if executed := rxp.TryExecute(ctx, &task); !executed {
//		promise.Fail(errors.From(ErrDial, errors.WithWrap(rxp.ErrBusy)))
//		return
//	}
//
//	return
//}
//
//func dialErrInterceptor(ctx context.Context, conn transport.Connection, err error) (future async.Future[transport.Connection]) {
//	if !errors.Is(err, ErrDial) {
//		err = errors.From(
//			ErrDial,
//			errors.WithWrap(err),
//		)
//	}
//	future = async.Immediately[transport.Connection](ctx, conn, err)
//	return
//}
//
//type dialer struct {
//	network string
//	address string
//	options DialOptions
//	promise async.Promise[transport.Connection]
//}
//
//func (d *dialer) Handle(ctx context.Context) {
//	network, address, options := d.network, d.address, d.options
//	connectOpts := aio.ConnectOptions{
//		MultipathTCP: options.MultipathTCP,
//		LocalAddr:    options.LocalAddr,
//		FastOpen:     options.FastOpen,
//	}
//	promise := d.promise
//	aio.Connect(network, address, connectOpts, func(userdata aio.Userdata, err error) {
//		if err != nil {
//			err = errors.From(
//				ErrDial,
//				errors.WithWrap(err),
//			)
//			promise.Fail(err)
//			return
//		}
//		connFd := userdata.Fd.(aio.NetFd)
//		var conn transport.Connection
//
//		switch network {
//		case "tcp", "tcp4", "tcp6":
//			conn = newTCPConnection(ctx, connFd)
//			break
//		case "udp", "udp4", "udp6":
//			conn = newPacketConnection(ctx, connFd)
//			break
//		case "unix", "unixgram", "unixpacket":
//			if network == "unix" {
//				conn = newTCPConnection(ctx, connFd)
//			} else {
//				conn = newPacketConnection(ctx, connFd)
//			}
//			break
//		case "ip", "ip4", "ip6":
//			conn = newPacketConnection(ctx, connFd)
//			break
//		default:
//			// not matched, so close it
//			_ = aio.Close(connFd)
//			promise.Fail(errors.From(ErrNetworkUnmatched))
//			return
//		}
//
//		if n := options.DefaultConnReadTimeout; n > 0 {
//			conn.SetReadTimeout(n)
//		}
//		if n := options.DefaultConnWriteTimeout; n > 0 {
//			conn.SetWriteTimeout(n)
//		}
//		if n := options.DefaultConnReadBufferSize; n > 0 {
//			err = conn.SetReadBuffer(n)
//			if err != nil {
//				_ = conn.Close()
//				promise.Fail(err)
//				return
//			}
//		}
//		if n := options.DefaultConnWriteBufferSize; n > 0 {
//			err = conn.SetWriteBuffer(n)
//			if err != nil {
//				_ = conn.Close()
//				promise.Fail(err)
//				return
//			}
//		}
//		if n := options.DefaultInboundBufferSize; n > 0 {
//			conn.SetInboundBuffer(n)
//		}
//
//		// tls
//		if options.TLSConnectionBuilder != nil {
//			conn = options.TLSConnectionBuilder.Client(conn)
//		}
//		if !promise.Succeed(conn) {
//			_ = conn.Close()
//		}
//		return
//	})
//}
