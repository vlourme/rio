package adaptor

import (
	"context"
	"github.com/brickingsoft/rio/transport"
	"net"
	"sync"
)

func Listener(ln transport.Listener) net.Listener {
	return &listener{
		ln:   ln,
		ach:  make(chan acceptResult, 1),
		once: sync.Once{},
	}
}

type acceptResult struct {
	conn net.Conn
	err  error
}

type listener struct {
	ln   transport.Listener
	ach  chan acceptResult
	once sync.Once
}

func (ln *listener) Accept() (net.Conn, error) {
	ln.once.Do(func() {
		ln.ln.OnAccept(func(ctx context.Context, conn transport.Connection, err error) {
			if err != nil {
				ln.ach <- acceptResult{
					conn: nil,
					err:  err,
				}
				return
			}
			ln.ach <- acceptResult{
				conn: Connection(conn),
				err:  nil,
			}
			return
		})
	})
	r := <-ln.ach
	return r.conn, r.err
}

func (ln *listener) Close() error {
	return ln.ln.Close()
}

func (ln *listener) Addr() net.Addr {
	return ln.ln.Addr()
}
