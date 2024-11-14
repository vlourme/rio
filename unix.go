package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/rate/timeslimiter"
	"github.com/brickingsoft/rio/pkg/sockets"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"net"
	"time"
)

// tcp: unix,unixpacket
// udp: unixgram

type UnixConnection interface {
	PacketConnection
}

func newUnixConnection(ctx context.Context, conn sockets.Connection) (uc *unixConnection) {
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

func (conn *unixConnection) ReadMsg() (future async.Future[transport.MsgInbound]) {
	//TODO implement me
	panic("implement me")
}

func (conn *unixConnection) WriteMsg(p []byte, oob []byte, addr net.Addr) (future async.Future[transport.MsgOutbound]) {
	//TODO implement me
	panic("implement me")
}
