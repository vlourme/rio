package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/async"
	"github.com/brickingsoft/rio/pkg/bytebufferpool"
	"net"
	"time"
)

type Inbound interface {
	Buffer() (buf bytebufferpool.Buffer)
	Received() (n int)
}

type inbound struct {
	buf bytebufferpool.Buffer
	n   int
}

func (in *inbound) Buffer() (buf bytebufferpool.Buffer) {
	buf = in.buf
	return
}

func (in *inbound) Received() (n int) {
	n = in.n
	return
}

type Outbound interface {
	Bytes() (p []byte)
	Wrote() (n int)
}

type outbound struct {
	p []byte
	n int
}

func (out *outbound) Bytes() (p []byte) {
	p = out.p
	return
}

func (out *outbound) Wrote() (n int) {
	n = out.n
	return
}

type Connection interface {
	Context() (ctx context.Context)
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

type ConnectionOnClose func(conn Connection)
