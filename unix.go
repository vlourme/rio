package rio

import (
	"context"
	"crypto/tls"
	"github.com/brickingsoft/rio/async"
	"github.com/brickingsoft/rio/pkg/maxprocs"
	"github.com/brickingsoft/rio/pkg/rate/timeslimiter"
	"github.com/brickingsoft/rio/pkg/security"
	"github.com/brickingsoft/rio/pkg/sockets"
	"github.com/brickingsoft/rio/transport"
	"net"
	"time"
)

// tcp: unix,unixpacket
// udp: unixgram

type UnixConnection interface {
	Connection
	PacketConnection
	ReadMsgUnix() (future async.Future[transport.UnixMsgInbound])
	WriteMsgUnix(b, oob []byte, addr *net.UnixAddr) (future async.Future[transport.MsgOutbound])
}

func newUnixConnection(ctx context.Context, conn sockets.UnixConnection) (uc *unixConnection) {

	return
}

type unixConnection struct {
	ctx context.Context
}

func (conn *unixConnection) Context() (ctx context.Context) {
	//TODO implement me
	panic("implement me")
}

func (conn *unixConnection) LocalAddr() (addr net.Addr) {
	//TODO implement me
	panic("implement me")
}

func (conn *unixConnection) RemoteAddr() (addr net.Addr) {
	//TODO implement me
	panic("implement me")
}

func (conn *unixConnection) SetDeadline(t time.Time) (err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *unixConnection) SetReadDeadline(t time.Time) (err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *unixConnection) SetWriteDeadline(t time.Time) (err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *unixConnection) SetReadBufferSize(size int) {
	//TODO implement me
	panic("implement me")
}

func (conn *unixConnection) Read() (future async.Future[transport.Inbound]) {
	//TODO implement me
	panic("implement me")
}

func (conn *unixConnection) Write(p []byte) (future async.Future[transport.Outbound]) {
	//TODO implement me
	panic("implement me")
}

func (conn *unixConnection) Close() (err error) {
	timeslimiter.Revert(conn.ctx)
	//TODO implement me
	panic("implement me")
}

func (conn *unixConnection) ReadFrom() (future async.Future[transport.PacketInbound]) {
	//TODO implement me
	panic("implement me")
}

func (conn *unixConnection) WriteTo(p []byte, addr net.Addr) (future async.Future[transport.Outbound]) {
	//TODO implement me
	panic("implement me")
}

func (conn *unixConnection) ReadMsgUnix() (future async.Future[transport.UnixMsgInbound]) {
	//TODO implement me
	panic("implement me")
}

func (conn *unixConnection) WriteToUnix(b []byte, addr *net.UnixAddr) (future async.Future[transport.Outbound]) {
	//TODO implement me
	panic("implement me")
}

func (conn *unixConnection) WriteMsgUnix(b, oob []byte, addr *net.UnixAddr) (future async.Future[transport.MsgOutbound]) {
	//TODO implement me
	panic("implement me")
}

type unixListener struct {
	ctx                           context.Context
	cancel                        context.CancelFunc
	inner                         sockets.UnixListener
	connectionsLimiter            *timeslimiter.Bucket
	connectionsLimiterWaitTimeout time.Duration
	executors                     async.Executors
	tlsConfig                     *tls.Config
	promises                      []async.Promise[Connection]
	maxprocsUndo                  maxprocs.Undo
}

func (ln *unixListener) Addr() (addr net.Addr) {
	addr = ln.inner.Addr()
	return
}

func (ln *unixListener) Accept() (future async.Future[Connection]) {
	ctx := ln.ctx
	promisesLen := len(ln.promises)
	for i := 0; i < promisesLen; i++ {
		promise, promiseErr := async.MustStreamPromise[Connection](ctx, 8)
		if promiseErr != nil {
			future = async.FailedImmediately[Connection](ctx, promiseErr)
			_ = ln.Close()
			return
		}
		ln.acceptOne(promise, 0)
		ln.promises[i] = promise
	}
	future = async.Group[Connection](ln.promises)
	return
}

func (ln *unixListener) Close() (err error) {
	for _, promise := range ln.promises {
		promise.Cancel()
	}
	ln.cancel()
	err = ln.inner.Close()
	ln.executors.CloseGracefully()
	ln.maxprocsUndo()
	return
}

func (ln *unixListener) ok() bool {
	return ln.ctx.Err() == nil
}

func (ln *unixListener) acceptOne(streamPromise async.Promise[Connection], limitedTimes int) {
	if !ln.ok() {
		streamPromise.Fail(ErrClosed)
		return
	}
	if limitedTimes > 9 {
		time.Sleep(ms10)
		limitedTimes = 0
	}
	ctx, cancel := context.WithTimeout(ln.ctx, ln.connectionsLimiterWaitTimeout)
	waitErr := ln.connectionsLimiter.Wait(ctx)
	cancel()
	if waitErr != nil {
		limitedTimes++
		ln.acceptOne(streamPromise, limitedTimes)
		return
	}
	ln.inner.AcceptUnix(func(sock sockets.UnixConnection, err error) {
		if err != nil {
			streamPromise.Fail(err)
			return
		}
		if ln.tlsConfig != nil {
			sock = security.Serve(ln.ctx, sock, ln.tlsConfig).(sockets.UnixConnection)
		}
		conn := newUnixConnection(ln.ctx, sock)
		streamPromise.Succeed(conn)
		ln.acceptOne(streamPromise, 0)
		return
	})
}
