package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"github.com/brickingsoft/rio/pkg/sys"
	"io"
	"net"
	"sync/atomic"
	"syscall"
	"time"
)

func newTcpConnection(ctx context.Context, vortex *aio.Vortex, fd *sys.Fd) *tcpConnection {
	cc, cancel := context.WithCancel(ctx)
	return &tcpConnection{
		connection{
			ctx:          cc,
			cancel:       cancel,
			fd:           fd,
			vortex:       vortex,
			readTimeout:  atomic.Int64{},
			writeTimeout: atomic.Int64{},
		},
	}
}

type tcpConnection struct {
	connection
}

func (conn *tcpConnection) SyscallConn() (syscall.RawConn, error) {
	return newRawConnection(conn.fd), nil
}

func (conn *tcpConnection) ReadFrom(r io.Reader) (int64, error) {
	return 0, &net.OpError{Op: "readfrom", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: nil}
}

func (conn *tcpConnection) WriteTo(w io.Writer) (int64, error) {
	return 0, &net.OpError{Op: "writeto", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: nil}
}

func (conn *tcpConnection) CloseRead() error {
	if err := conn.fd.CloseRead(); err != nil {
		return &net.OpError{Op: "close", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (conn *tcpConnection) CloseWrite() error {
	if err := conn.fd.CloseWrite(); err != nil {
		return &net.OpError{Op: "close", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (conn *tcpConnection) SetLinger(sec int) error {
	if err := conn.fd.SetLinger(sec); err != nil {
		return &net.OpError{Op: "set", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (conn *tcpConnection) SetNoDelay(noDelay bool) error {
	if err := conn.fd.SetNoDelay(noDelay); err != nil {
		return &net.OpError{Op: "set", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (conn *tcpConnection) SetKeepAlive(keepalive bool) error {
	if err := conn.fd.SetKeepAlive(keepalive); err != nil {
		return &net.OpError{Op: "set", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (conn *tcpConnection) SetKeepAlivePeriod(period time.Duration) error {
	if err := conn.fd.SetKeepAlivePeriod(period); err != nil {
		return &net.OpError{Op: "set", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (conn *tcpConnection) SetKeepAliveConfig(config net.KeepAliveConfig) error {
	if err := conn.fd.SetKeepAliveConfig(config); err != nil {
		return &net.OpError{Op: "set", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (conn *tcpConnection) MultipathTCP() (bool, error) {
	ok := sys.IsUsingMultipathTCP(conn.fd)
	return ok, nil
}

//
//func newTCPConnection(ctx context.Context, fd aio.NetFd) (conn transport.TCPConnection) {
//	conn = &tcpConnection{
//		connection: connection{
//			ctx:    ctx,
//			fd:     fd,
//			closed: &atomic.Bool{},
//			rb:     bytebuffers.Acquire(),
//			rbs:    defaultReadBufferSize,
//		},
//	}
//	return
//}
//
//type tcpConnection struct {
//	connection
//}
//
//func (conn *tcpConnection) MultipathTCP() bool {
//	return aio.IsUsingMultipathTCP(conn.fd)
//}
//
//func (conn *tcpConnection) SetNoDelay(noDelay bool) (err error) {
//	err = aio.SetNoDelay(conn.fd, noDelay)
//	return
//}
//
//func (conn *tcpConnection) SetLinger(sec int) (err error) {
//	err = aio.SetLinger(conn.fd, sec)
//	return
//}
//
//func (conn *tcpConnection) SetKeepAlive(keepalive bool) (err error) {
//	err = aio.SetKeepAlive(conn.fd, keepalive)
//	return
//}
//
//func (conn *tcpConnection) SetKeepAlivePeriod(period time.Duration) (err error) {
//	err = aio.SetKeepAlivePeriod(conn.fd, period)
//	return
//}
//
//func (conn *tcpConnection) SetKeepAliveConfig(config aio.KeepAliveConfig) (err error) {
//	err = aio.SetKeepAliveConfig(conn.fd, config)
//	return
//}
//
//func (conn *tcpConnection) Sendfile(file string) (future async.Future[int]) {
//	if len(file) == 0 {
//		err := errors.From(
//			ErrSendfile,
//			errors.WithWrap(errors.Define("no file specified")),
//		)
//		future = async.FailedImmediately[int](conn.ctx, err)
//		return
//	}
//	if conn.disconnected() {
//		err := errors.From(
//			ErrSendfile,
//			errors.WithWrap(ErrClosed),
//		)
//		future = async.FailedImmediately[int](conn.ctx, err)
//		return
//	}
//
//	var promise async.Promise[int]
//	var promiseErr error
//	if timeout := conn.writeTimeout; timeout > 0 {
//		promise, promiseErr = async.Make[int](conn.ctx, async.WithTimeout(timeout))
//	} else {
//		promise, promiseErr = async.Make[int](conn.ctx)
//	}
//	if promiseErr != nil {
//		err := errors.From(
//			ErrSendfile,
//			errors.WithWrap(promiseErr),
//		)
//		future = async.FailedImmediately[int](conn.ctx, err)
//		return
//	}
//	promise.SetErrInterceptor(conn.sendfileErrInterceptor)
//
//	aio.Sendfile(conn.fd, file, func(userdata aio.Userdata, err error) {
//		n := userdata.N
//		if err != nil {
//			err = errors.From(
//				ErrSendfile,
//				errors.WithWrap(err),
//			)
//		}
//		promise.Complete(n, err)
//		return
//	})
//
//	future = promise.Future()
//	return
//}
//
//func (conn *connection) sendfileErrInterceptor(ctx context.Context, n int, err error) (future async.Future[int]) {
//	if !errors.Is(err, ErrSendfile) {
//		err = errors.From(
//			ErrSendfile,
//			errors.WithWrap(err),
//		)
//	}
//	future = async.Immediately[int](ctx, n, err)
//	return
//}
