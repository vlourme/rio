package rio

import (
	"context"
	"crypto/tls"
	"github.com/brickingsoft/rio/pkg/async"
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
	// ReadFromUnix acts like [UnixConn.ReadFrom] but returns a [UnixAddr].
	ReadFromUnix() (future async.Future[transport.UnixInbound])
	// ReadMsgUnix reads a message from c, copying the payload into b and
	// the associated out-of-band data into oob. It returns the number of
	// bytes copied into b, the number of bytes copied into oob, the flags
	// that were set on the message and the source address of the message.
	//
	// Note that if len(b) == 0 and len(oob) > 0, this function will still
	// read (and discard) 1 byte from the connection.
	ReadMsgUnix() (future async.Future[transport.UnixMsgInbound])
	// WriteToUnix acts like [UnixConn.WriteTo] but takes a [UnixAddr].
	WriteToUnix(b []byte, addr *net.UnixAddr) (future async.Future[transport.Outbound])
	// WriteMsgUnix writes a message to addr via c, copying the payload
	// from b and the associated out-of-band data from oob. It returns the
	// number of payload and out-of-band bytes written.
	//
	// Note that if len(b) == 0 and len(oob) > 0, this function will still
	// write 1 byte to the connection.
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

func (conn *unixConnection) ReadFromUnix() (future async.Future[transport.UnixInbound]) {
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

func (ln *unixListener) Close() (err error) {
	for _, promise := range ln.promises {
		promise.Cancel()
	}
	ln.cancel()
	err = ln.inner.Close()
	ln.executors.GracefulClose()
	ln.maxprocsUndo()
	return
}

func (ln *unixListener) ok() bool {
	return ln.ctx.Err() == nil
}

func (ln *unixListener) acceptOne(infinitePromise async.Promise[Connection]) {
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
	ln.inner.AcceptUnix(func(sock sockets.UnixConnection, err error) {
		if err != nil {
			infinitePromise.Fail(err)
			return
		}
		if ln.tlsConfig != nil {
			sock = security.Serve(ln.ctx, sock, ln.tlsConfig).(sockets.UnixConnection)
		}
		conn := newUnixConnection(ln.ctx, sock)
		infinitePromise.Succeed(conn)
		ln.acceptOne(infinitePromise)
		return
	})
}
