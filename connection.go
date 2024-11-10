package rio

import (
	"context"
	"github.com/brickingsoft/rio/async"
	"github.com/brickingsoft/rio/transport"
	"net"
	"time"
)

type Connection interface {
	Context() (ctx context.Context)
	LocalAddr() (addr net.Addr)
	RemoteAddr() (addr net.Addr)
	SetDeadline(t time.Time) (err error)
	SetReadDeadline(t time.Time) (err error)
	SetWriteDeadline(t time.Time) (err error)
	SetReadBufferSize(size int)
	Read() (future async.Future[transport.Inbound])
	Write(p []byte) (future async.Future[transport.Outbound])
	Close() (err error)
}
