package transport

import (
	"github.com/brickingsoft/rxp/async"
	"time"
)

type Reader interface {
	Read() (future async.Future[Inbound])
}

type Writer interface {
	Write(b []byte) (future async.Future[int])
}

type Closer interface {
	Close() (future async.Future[async.Void])
}

type ReadWriter interface {
	Reader
	Writer
}

type ReadWriteCloser interface {
	Reader
	Writer
	Closer
}

type TimeoutReader interface {
	Reader
	SetReadTimeout(d time.Duration) (err error)
}

type TimeoutWriter interface {
	Reader
	SetWriteTimeout(d time.Duration) (err error)
}
