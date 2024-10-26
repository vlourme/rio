package rio

import (
	"github.com/brickingsoft/rio/pkg/async"
	"net"
)

func Listen() (ln Listener, err error) {

	return
}

type Listener interface {
	Addr() (addr net.Addr)
	Accept() (future async.Future[Connection])
	Close() (err error)
}
