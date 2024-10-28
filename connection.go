package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/async"
	"github.com/brickingsoft/rio/pkg/bytebufferpool"
	"net"
	"time"
)

type OperationMode int

func (op OperationMode) IsRead() bool {
	return op == Read
}

func (op OperationMode) IsWrite() bool {
	return op == Write
}

func (op OperationMode) String() string {
	switch op {
	case Read:
		return "read"
	case Write:
		return "write"
	default:
		return "unknown"
	}
}

const (
	Read OperationMode = iota + 1
	Write
)

type Operation interface {
	Connection() (conn Connection)
	Mode() (mode OperationMode)
	// Inbound
	// peek read discard
	Inbound() (buf bytebufferpool.Buffer)
	Outbound() (buf bytebufferpool.Buffer)
	Wrote() (n int)
	// RemoteAddr
	// used by ReadFrom
	RemoteAddr() (addr net.Addr)
}

type Inbound interface {
	context.Context
	Buffer() (buf bytebufferpool.Buffer)
	// RemoteAddr
	// used by ReadFrom
	RemoteAddr() (addr net.Addr)
}

type Outbound interface {
	context.Context
	Buffer() (buf bytebufferpool.Buffer)
	Wrote() (n int)
}

type Connection interface {
	context.Context
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
	ReadFrom() (future async.Future[Inbound])
	WriteTo(p []byte, addr net.Addr) (future async.Future[Outbound])
	Close() (err error)
}
