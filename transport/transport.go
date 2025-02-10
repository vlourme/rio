package transport

import (
	"context"
	"github.com/brickingsoft/rio/pkg/aio"
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

type PacketInbound struct {
	Bytes []byte
	Addr  net.Addr
}

type PacketMsgInbound struct {
	Bytes []byte
	OOB   []byte
	Flags int
	Addr  net.Addr
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
	InboundBuffer() int
	Read() (future async.Future[Inbound])
	Write(b []byte) (future async.Future[int])
	Close() (err error)
}

type TCPConnection interface {
	Connection
	Sendfile(file string) (future async.Future[int])
	MultipathTCP() bool
	SetNoDelay(noDelay bool) (err error)
	SetLinger(sec int) (err error)
	SetKeepAlive(keepalive bool) (err error)
	SetKeepAlivePeriod(period time.Duration) (err error)
	SetKeepAliveConfig(config aio.KeepAliveConfig) (err error)
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

type Listener interface {
	// Addr
	// 地址
	Addr() (addr net.Addr)
	// OnAccept
	// 准备接收一个链接。
	// 当服务关闭时，得到一个 async.Canceled 错误。可以使用 async.IsCanceled 进行判断。
	// 当得到错误时，务必不要退出 OnAccept，请以是否收到 async.Canceled 来决定退出。
	// 不支持多次调用。
	OnAccept(fn func(ctx context.Context, conn Connection, err error))
	// Close
	// 关闭
	Close() (err error)
}
