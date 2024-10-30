package rio

import (
	"github.com/brickingsoft/rio/pkg/async"
	"github.com/brickingsoft/rio/pkg/bytebufferpool"
	"net"
	"time"
)

type Inbound interface {
	Buffer() (buf bytebufferpool.Buffer)
	Bytes() (n int)
}

type Outbound interface {
	Buffer() (buf bytebufferpool.Buffer)
	Wrote() (n int)
}

type Connection interface {
	LocalAddr() (addr net.Addr)
	RemoteAddr() (addr net.Addr)
	SetDeadline(t time.Time) (err error)
	SetReadDeadline(t time.Time) (err error)
	SetWriteDeadline(t time.Time) (err error)
	SetReadBufferSize(size int)
	Read() (future async.Future[Inbound])
	Write(p []byte) (future async.Future[Outbound])
	Close() (err error)
}
