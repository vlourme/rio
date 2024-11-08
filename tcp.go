package rio

import (
	"context"
	"crypto/tls"
	"errors"
	"github.com/brickingsoft/rio/pkg/async"
	"github.com/brickingsoft/rio/pkg/maxprocs"
	"github.com/brickingsoft/rio/pkg/rate/timeslimiter"
	"github.com/brickingsoft/rio/pkg/security"
	"github.com/brickingsoft/rio/pkg/sockets"
	"net"
	"time"
)

type TCPConnection interface {
	Connection
	SetNoDelay(noDelay bool) (err error)
	SetLinger(sec int) (err error)
	SetKeepAlive(keepalive bool) (err error)
	SetKeepAlivePeriod(period time.Duration) (err error)
}

const (
	defaultRWTimeout      = 15 * time.Second
	defaultReadBufferSize = 1024
)

func newTCPConnection(ctx context.Context, inner sockets.TCPConnection) (conn TCPConnection) {
	connCtx, cancel := context.WithCancel(ctx)
	conn = &tcpConnection{
		ctx:    connCtx,
		cancel: cancel,
		inner:  inner,
		rb:     new(inboundBuffer),
		rbs:    defaultReadBufferSize,
		rto:    defaultRWTimeout,
		wto:    defaultRWTimeout,
	}
	return
}

type tcpConnection struct {
	ctx    context.Context
	cancel context.CancelFunc
	inner  sockets.TCPConnection
	rb     *inboundBuffer
	rbs    int
	rto    time.Duration
	wto    time.Duration
}

func (conn *tcpConnection) Context() (ctx context.Context) {
	ctx = conn.ctx
	return
}

func (conn *tcpConnection) LocalAddr() (addr net.Addr) {
	addr = conn.inner.LocalAddr()
	return
}

func (conn *tcpConnection) RemoteAddr() (addr net.Addr) {
	addr = conn.inner.RemoteAddr()
	return
}

func (conn *tcpConnection) SetDeadline(t time.Time) (err error) {
	timeout := time.Until(t)
	if timeout < 1 {
		err = errors.New("deadline too short")
		return
	}
	err = conn.inner.SetDeadline(t)
	if err != nil {
		return
	}
	conn.rto = timeout
	conn.wto = timeout
	return
}

func (conn *tcpConnection) SetReadDeadline(t time.Time) (err error) {
	timeout := time.Until(t)
	if timeout < 1 {
		err = errors.New("deadline too short")
		return
	}
	err = conn.inner.SetReadDeadline(t)
	if err != nil {
		return
	}
	conn.rto = timeout
	return
}

func (conn *tcpConnection) SetWriteDeadline(t time.Time) (err error) {
	timeout := time.Until(t)
	if timeout < 1 {
		err = errors.New("deadline too short")
		return
	}
	err = conn.inner.SetWriteDeadline(t)
	if err != nil {
		return
	}
	conn.wto = timeout
	return
}

func (conn *tcpConnection) SetReadBufferSize(size int) {
	if size < 1 {
		size = defaultReadBufferSize
	}
	conn.rbs = size
	return
}

func (conn *tcpConnection) Read() (future async.Future[Inbound]) {
	promise, ok := async.TryPromise[Inbound](conn.ctx)
	if !ok {
		future = async.FailedImmediately[Inbound](conn.ctx, ErrBusy)
		return
	}
	timeout := time.Now().Add(conn.rto)
	promise.SetDeadline(timeout)
	p := conn.rb.allocate(conn.rbs)
	conn.inner.Read(p, func(n int, err error) {
		conn.rb.free()
		if err != nil {
			promise.Fail(err)
			return
		}
		promise.Succeed(inbound{
			buf: conn.rb,
			n:   n,
		})
		return
	})
	future = promise.Future()
	return
}

func (conn *tcpConnection) Write(p []byte) (future async.Future[Outbound]) {
	if len(p) == 0 {
		future = async.FailedImmediately[Outbound](conn.ctx, ErrEmptyPacket)
		return
	}

	promise, ok := async.TryPromise[Outbound](conn.ctx)
	if !ok {
		future = async.FailedImmediately[Outbound](conn.ctx, ErrBusy)
		return
	}

	timeout := time.Now().Add(conn.wto)
	promise.SetDeadline(timeout)

	conn.write(p, 0, promise)

	future = promise.Future()
	return
}

func (conn *tcpConnection) write(p []byte, wrote int, promise async.Promise[Outbound]) {
	if err := conn.ctx.Err(); err != nil {
		promise.Succeed(outbound{
			n:   wrote,
			err: err,
		})
		return
	}
	conn.inner.Write(p, func(n int, err error) {
		if err != nil {
			if wrote == 0 {
				promise.Fail(err)
			} else {
				promise.Succeed(outbound{
					n:   wrote,
					err: err,
				})
			}
			return
		}
		if n == len(p) {
			promise.Succeed(outbound{
				n:   wrote + n,
				err: nil,
			})
			return
		}
		conn.write(p[n:], wrote+n, promise)
		return
	})
	return
}

func (conn *tcpConnection) Close() (err error) {
	conn.cancel()
	err = conn.inner.Close()
	conn.rb.tryRelease()
	timeslimiter.Revert(conn.ctx)
	return
}

func (conn *tcpConnection) SetNoDelay(noDelay bool) (err error) {
	err = conn.inner.SetNoDelay(noDelay)
	return
}

func (conn *tcpConnection) SetLinger(sec int) (err error) {
	err = conn.inner.SetLinger(sec)
	return
}

func (conn *tcpConnection) SetKeepAlive(keepalive bool) (err error) {
	err = conn.inner.SetKeepAlive(keepalive)
	return
}

func (conn *tcpConnection) SetKeepAlivePeriod(period time.Duration) (err error) {
	err = conn.inner.SetKeepAlivePeriod(period)
	return
}

type tcpListener struct {
	ctx                           context.Context
	cancel                        context.CancelFunc
	inner                         sockets.TCPListener
	connectionsLimiter            *timeslimiter.Bucket
	connectionsLimiterWaitTimeout time.Duration
	executors                     async.Executors
	tlsConfig                     *tls.Config
	promises                      []async.Promise[Connection]
	maxprocsUndo                  maxprocs.Undo
}

func (ln *tcpListener) Addr() (addr net.Addr) {
	addr = ln.inner.Addr()
	return
}

func (ln *tcpListener) Accept() (future async.Future[Connection]) {
	ctx := ln.ctx
	promisesLen := len(ln.promises)
	for i := 0; i < promisesLen; i++ {
		promise, promiseErr := async.MustInfinitePromise[Connection](ctx, 8)
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

func (ln *tcpListener) Close() (err error) {
	for _, promise := range ln.promises {
		promise.Cancel()
	}
	ln.cancel()
	err = ln.inner.Close()
	ln.executors.GracefulClose()
	ln.maxprocsUndo()
	return
}

func (ln *tcpListener) ok() bool {
	return ln.ctx.Err() == nil
}

func (ln *tcpListener) acceptOne(infinitePromise async.Promise[Connection]) {
	if !ln.ok() {
		infinitePromise.Fail(ErrClosed)
		return
	}
	ctx, cancel := context.WithTimeout(ln.ctx, ln.connectionsLimiterWaitTimeout)
	waitErr := ln.connectionsLimiter.Wait(ctx)
	cancel()
	if waitErr != nil {
		ln.acceptOne(infinitePromise)
		return
	}
	ln.inner.Accept(func(sock sockets.TCPConnection, err error) {
		if err != nil {
			infinitePromise.Fail(err)
			return
		}
		if ln.tlsConfig != nil {
			sock = security.Serve(ln.ctx, sock, ln.tlsConfig).(sockets.TCPConnection)
		}
		conn := newTCPConnection(ln.ctx, sock)
		infinitePromise.Succeed(conn)
		ln.acceptOne(infinitePromise)
		return
	})
}
