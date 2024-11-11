package rio

import (
	"context"
	"github.com/brickingsoft/rio/async"
	"github.com/brickingsoft/rio/transport"
	"net"
)

// ListenPacket
// "udp", "udp4", "udp6", "unixgram"
func ListenPacket(ctx context.Context, network string, addr string, options ...Option) (conn PacketConnection, err error) {

	return
}

type PacketConnection interface {
	Connection
	ReadFrom() (future async.Future[transport.PacketInbound])
	WriteTo(p []byte, addr net.Addr) (future async.Future[transport.Outbound])
}
