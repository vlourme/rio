package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/async"
	"github.com/brickingsoft/rio/pkg/bytebufferpool"
	"net"
	"time"
)

// ListenPacket
// "udp", "udp4", "udp6", "unixgram"
func ListenPacket(ctx context.Context, network string, addr string, options ...Option) (conn PacketConnection, err error) {

	return
}

type PacketInbound interface {
	Buffer() (buf bytebufferpool.Buffer)
	Bytes() (n int)
	Addr() (addr net.Addr)
}

type PacketConnection interface {
	LocalAddr() (addr net.Addr)
	SetDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	SetReadBufferSize(size int)
	ReadFrom() (future async.Future[PacketInbound])
	WriteTo(p []byte, addr net.Addr) (future async.Future[Outbound])
	Close() error
}
