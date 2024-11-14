package rio

import (
	"context"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"net"
)

// ListenPacket
// "udp", "udp4", "udp6", "unixgram", "ip"
func ListenPacket(ctx context.Context, network string, addr string, options ...Option) (conn PacketConnection, err error) {

	return
}

type PacketConnection interface {
	Connection
	ReadFrom() (future async.Future[transport.PacketInbound])
	WriteTo(p []byte, addr net.Addr) (future async.Future[transport.Outbound])
	SetReadMsgUDPOOBBufferSize(size int)
	ReadMsg() (future async.Future[transport.MsgInbound])
	WriteMsg(p []byte, oob []byte, addr net.Addr) (future async.Future[transport.MsgOutbound])
}
