package transport

import (
	"context"
	"github.com/brickingsoft/rxp/async"
	"net"
	"time"
)

type Inbound interface {
	Peek(n int) (b []byte)
	Next(n int) (b []byte, err error)
	Read(b []byte) (n int, err error)
	Discard(n int)
	Len() (n int)
	ReadBytes(delim byte) (line []byte, err error)
	Index(delim byte) (i int)
}

type PacketInbound interface {
	Inbound
	Addr() (addr net.Addr)
}

type PacketMsgInbound interface {
	Inbound
	OOB() (oob []byte)
	Flags() (n int)
	Addr() (addr net.Addr)
}

type PacketMsgOutbound struct {
	N    int
	OOBN int
}

type Reader interface {
	Read() (future async.Future[Inbound])
}

type Writer interface {
	Write(b []byte) (future async.Future[int])
}

type Connection interface {
	Context() (ctx context.Context)
	ConfigContext(config func(ctx context.Context) context.Context)
	Fd() int
	LocalAddr() (addr net.Addr)
	RemoteAddr() (addr net.Addr)
	SetReadTimeout(d time.Duration)
	SetWriteTimeout(d time.Duration)
	SetReadBuffer(n int) (err error)
	SetWriteBuffer(n int) (err error)
	SetInboundBuffer(n int)
	Read() (future async.Future[Inbound])
	Write(b []byte) (future async.Future[int])
	Close() (err error)
}

type PacketReader interface {
	ReadFrom() (future async.Future[PacketInbound])
}

type PacketWriter interface {
	WriteTo(b []byte, addr net.Addr) (future async.Future[int])
}

type PacketConnection interface {
	Connection
	ReadFrom() (future async.Future[PacketInbound])
	WriteTo(b []byte, addr net.Addr) (future async.Future[int])
	SetReadMsgOOBBufferSize(size int)
	ReadMsg() (future async.Future[PacketMsgInbound])
	WriteMsg(b []byte, oob []byte, addr net.Addr) (future async.Future[PacketMsgOutbound])
}
