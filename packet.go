package rio

import (
	"context"
	"net"
	"time"
)

// ListenPacket
func ListenPacket(ctx context.Context, network string, addr string, options ...Option) (conn PacketConn, err error) {

	return
}

type PacketConn interface {
	ReadFrom(p []byte) (n int, addr net.Addr, err error)
	WriteTo(p []byte, addr net.Addr) (n int, err error)
	Close() error
	SetDeadline(t time.Time) error
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
}
