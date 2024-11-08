package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/async"
	"github.com/brickingsoft/rio/pkg/bytebufferpool"
	"net"
	"time"
)

type InboundBuffer interface {
	Peek(n int) (p []byte)
	Next(n int) (p []byte, err error)
	Read(p []byte) (n int, err error)
	Discard(n int)
}

type inboundBuffer struct {
	b    bytebufferpool.Buffer
	area bytebufferpool.AreaOfBuffer
}

func (buf *inboundBuffer) allocate(size int) (p []byte) {
	if buf.area != nil {
		panic("rio: buffer already allocated a piece bytes")
		return
	}
	if buf.b == nil {
		buf.b = bytebufferpool.Get()
	}
	buf.area = buf.b.ApplyAreaForWrite(size)
	p = buf.area.Bytes()
	return
}

func (buf *inboundBuffer) free() {
	if buf.area != nil {
		buf.area.Finish()
		buf.area = nil
	}
}

func (buf *inboundBuffer) tryRelease() {
	if buf.area != nil {
		buf.area.Cancel()
		buf.area = nil
	}
	if buf.b != nil {
		bytebufferpool.Put(buf.b)
		buf.b = nil
	}
}

func (buf *inboundBuffer) Peek(n int) (p []byte) {
	if buf.b == nil {
		return
	}
	p = buf.b.Peek(n)
	return
}

func (buf *inboundBuffer) Next(n int) (p []byte, err error) {
	if buf.b == nil {
		return
	}
	p, err = buf.b.Next(n)
	if buf.b.Len() == 0 {
		bytebufferpool.Put(buf.b)
		buf.b = nil
	}
	return
}

func (buf *inboundBuffer) Read(p []byte) (n int, err error) {
	if buf.b == nil {
		return
	}
	n, err = buf.b.Read(p)
	if buf.b.Len() == 0 {
		bytebufferpool.Put(buf.b)
		buf.b = nil
	}
	return
}

func (buf *inboundBuffer) Discard(n int) {
	if buf.b == nil {
		return
	}
	buf.b.Discard(n)
	if buf.b.Len() == 0 {
		bytebufferpool.Put(buf.b)
		buf.b = nil
	}
	return
}

type Inbound interface {
	Buffer() (buf InboundBuffer)
	Received() (n int)
}

type inbound struct {
	buf InboundBuffer
	n   int
}

func (in inbound) Buffer() (buf InboundBuffer) {
	buf = in.buf
	return
}

func (in inbound) Received() (n int) {
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

func (out outbound) Bytes() (p []byte) {
	p = out.p
	return
}

func (out outbound) Wrote() (n int) {
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
