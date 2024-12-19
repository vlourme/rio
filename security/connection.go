package security

import (
	"context"
	"crypto/tls"
	"github.com/brickingsoft/rio/pkg/aio"
	"github.com/brickingsoft/rio/pkg/rate/timeslimiter"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"net"
	"time"
)

type ConnectionBuilder func(ctx context.Context, fd aio.NetFd, config *tls.Config) (conn Connection, err error)

type Connection interface {
	Context() (ctx context.Context)
	ConfigContext(config func(ctx context.Context) context.Context)
	Fd() int
	LocalAddr() (addr net.Addr)
	RemoteAddr() (addr net.Addr)
	SetReadTimeout(d time.Duration) (err error)
	SetWriteTimeout(d time.Duration) (err error)
	SetReadBuffer(n int) (err error)
	SetWriteBuffer(n int) (err error)
	SetInboundBuffer(n int)
	MultipathTCP() bool
	SetNoDelay(noDelay bool) (err error)
	SetLinger(sec int) (err error)
	SetKeepAlive(keepalive bool) (err error)
	SetKeepAlivePeriod(period time.Duration) (err error)
	SetKeepAliveConfig(config aio.KeepAliveConfig) (err error)
	Read() (future async.Future[transport.Inbound])
	Write(b []byte) (future async.Future[transport.Outbound])
	Close() (future async.Future[async.Void])
}

type TLSConnection struct {
	ctx context.Context
	fd  aio.NetFd
	rb  transport.InboundBuffer
	rbs int
}

func (conn *TLSConnection) Context() context.Context {
	return conn.ctx
}

func (conn *TLSConnection) ConfigContext(config func(ctx context.Context) context.Context) {
	if config == nil {
		return
	}
	newCtx := config(conn.ctx)
	if newCtx == nil {
		return
	}
	conn.ctx = newCtx
	return
}

func (conn *TLSConnection) Fd() int {
	return conn.fd.Fd()
}

func (conn *TLSConnection) LocalAddr() (addr net.Addr) {
	addr = conn.fd.LocalAddr()
	return
}

func (conn *TLSConnection) RemoteAddr() (addr net.Addr) {
	addr = conn.fd.RemoteAddr()
	return
}

func (conn *TLSConnection) SetReadTimeout(d time.Duration) (err error) {
	conn.fd.SetReadTimeout(d)
	return
}

func (conn *TLSConnection) SetWriteTimeout(d time.Duration) (err error) {
	conn.fd.SetWriteTimeout(d)
	return
}

func (conn *TLSConnection) SetReadBuffer(n int) (err error) {
	if err = aio.SetReadBuffer(conn.fd, n); err != nil {
		err = aio.NewOpErr(aio.OpSet, conn.fd, err)
		return
	}
	return
}

func (conn *TLSConnection) SetWriteBuffer(n int) (err error) {
	if err = aio.SetWriteBuffer(conn.fd, n); err != nil {
		err = aio.NewOpErr(aio.OpSet, conn.fd, err)
		return
	}
	return
}

const (
	defaultReadBufferSize = 1024
)

func (conn *TLSConnection) SetInboundBuffer(n int) {
	if n < 1 {
		n = defaultReadBufferSize
	}
	conn.rbs = n
	return
}

func (conn *TLSConnection) Read() (future async.Future[transport.Inbound]) {
	//TODO implement me
	panic("implement me")
}

func (conn *TLSConnection) Write(b []byte) (future async.Future[transport.Outbound]) {
	//TODO implement me
	panic("implement me")
}

func (conn *TLSConnection) Sendfile(file string) (future async.Future[transport.Outbound]) {
	return
}

func (conn *TLSConnection) Close() (future async.Future[async.Void]) {
	promise, promiseErr := async.Make[async.Void](conn.ctx, async.WithUnlimitedMode())
	if promiseErr != nil {
		conn.rb.Close()
		aio.CloseImmediately(conn.fd)
		timeslimiter.TryRevert(conn.ctx)
		future = async.FailedImmediately[async.Void](conn.ctx, aio.NewOpErr(aio.OpClose, conn.fd, promiseErr))
		return
	}
	aio.Close(conn.fd, func(result int, userdata aio.Userdata, err error) {
		if err != nil {
			promise.Fail(aio.NewOpErr(aio.OpClose, conn.fd, err))
		} else {
			promise.Succeed(async.Void{})
		}
		conn.rb.Close()
		timeslimiter.TryRevert(conn.ctx)
		return
	})
	future = promise.Future()
	return
}

func (conn *TLSConnection) MultipathTCP() bool {
	return aio.IsUsingMultipathTCP(conn.fd)
}

func (conn *TLSConnection) SetNoDelay(noDelay bool) (err error) {
	err = aio.SetNoDelay(conn.fd, noDelay)
	return
}

func (conn *TLSConnection) SetLinger(sec int) (err error) {
	err = aio.SetLinger(conn.fd, sec)
	return
}

func (conn *TLSConnection) SetKeepAlive(keepalive bool) (err error) {
	err = aio.SetKeepAlive(conn.fd, keepalive)
	return
}

func (conn *TLSConnection) SetKeepAlivePeriod(period time.Duration) (err error) {
	err = aio.SetKeepAlivePeriod(conn.fd, period)
	return
}

func (conn *TLSConnection) SetKeepAliveConfig(config aio.KeepAliveConfig) (err error) {
	err = aio.SetKeepAliveConfig(conn.fd, config)
	return
}
