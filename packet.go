package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/async"
	"github.com/brickingsoft/rio/transport"
	"net"
	"time"
)

// ListenPacket
// "udp", "udp4", "udp6", "unixgram"
func ListenPacket(ctx context.Context, network string, addr string, options ...Option) (conn PacketConnection, err error) {

	return
}

type PacketConnection interface {
	Context() (ctx context.Context)
	LocalAddr() (addr net.Addr)
	SetDeadline(deadline time.Time) error
	SetReadDeadline(deadline time.Time) error
	SetWriteDeadline(deadline time.Time) error
	SetReadBufferSize(size int)
	ReadFrom() (future async.Future[transport.PacketInbound])
	WriteTo(p []byte, addr net.Addr) (future async.Future[transport.Outbound])
	Close() error
}
