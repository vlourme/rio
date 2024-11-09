package transport

import "github.com/brickingsoft/rio/pkg/bytebufferpool"

type InboundReader interface {
	Peek(n int) (p []byte)
	Next(n int) (p []byte, err error)
	Read(p []byte) (n int, err error)
	Discard(n int)
	Length() (n int)
}

type InboundBuffer interface {
	InboundReader
	Allocate(size int) (p []byte)
	Write(p []byte) (n int, err error)
	Free()
	Close()
}

func NewInboundBuffer() InboundBuffer {
	return new(inboundBuffer)
}

type inboundBuffer struct {
	b    bytebufferpool.Buffer
	area bytebufferpool.AreaOfBuffer
}

func (buf *inboundBuffer) Allocate(size int) (p []byte) {
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

func (buf *inboundBuffer) Free() {
	if buf.area != nil {
		buf.area.Finish()
		buf.area = nil
	}
}

func (buf *inboundBuffer) Write(p []byte) (n int, err error) {
	if buf.b == nil {
		buf.b = bytebufferpool.Get()
	}
	n, err = buf.b.Write(p)
	return
}

func (buf *inboundBuffer) Close() {
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
	if buf.b.Len() == 0 && !buf.b.WritePending() {
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
	if buf.b.Len() == 0 && !buf.b.WritePending() {
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

func (buf *inboundBuffer) Length() (n int) {
	if buf.b == nil {
		return
	}
	n = buf.b.Len()
	return
}

type Inbound interface {
	Reader() (buf InboundReader)
	Received() (n int)
}

func NewInbound(r InboundReader, n int) Inbound {
	return &inbound{
		r: r,
		n: n,
	}
}

type inbound struct {
	r InboundReader
	n int
}

func (in inbound) Reader() (r InboundReader) {
	r = in.r
	return
}

func (in inbound) Received() (n int) {
	n = in.n
	return
}
