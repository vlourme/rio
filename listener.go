package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/async"
	"net"
)

var (
	ErrBusy        = errors.New("rio: system busy")
	ErrEmptyPacket = errors.New("rio: empty packet")
)

// Listen
// ctx as root ctx, each conn can read it.
func Listen(ctx context.Context, network string, addr string, options ...Option) (ln Listener, err error) {

	return
}

type Listener interface {
	Addr() (addr net.Addr)
	Accept() (future async.Future[Connection])
	Close() (err error)
}
