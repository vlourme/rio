package security

import (
	"context"
	"crypto/tls"
	"github.com/brickingsoft/rio/pkg/sockets"
)

func Serve(conn sockets.Connection, config *tls.Config) (sc *Connection) {

	return nil
}

func Client(conn sockets.Connection, config *tls.Config) (sc *Connection) {

	return nil
}

type handshakeFn func(ctx context.Context, handler func(cause error))

type Connection struct {
	// impl sockets Conn
}
