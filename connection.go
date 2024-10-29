package rio

import (
	"github.com/brickingsoft/rio/pkg/async"
	"github.com/brickingsoft/rio/pkg/bytebufferpool"
	"net"
	"time"
)

type Inbound interface {
	Buffer() (buf bytebufferpool.Buffer)
	// RemoteAddr
	// used by ReadFrom
	RemoteAddr() (addr net.Addr)
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
	// Read
	// post request with userdata(promise)
	// in get status loop, get a result, get promise from userdata(max size is 64, such as ptr * 8, maybe one ptr + 7 padding), then emit an executor to complete promise
	Read() (future async.Future[Inbound])
	Write(p []byte) (future async.Future[Outbound])
	WriteTo(p []byte, addr net.Addr) (future async.Future[Outbound])
	Close() (err error)
}
