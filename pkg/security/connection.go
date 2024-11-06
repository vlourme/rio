package security

import (
	"context"
	"crypto/tls"
	"github.com/brickingsoft/rio/pkg/sockets"
)

func Serve(ctx context.Context, conn sockets.Connection, config *tls.Config) (sc sockets.TCPConnection) {

	return nil
}

func Client(ctx context.Context, conn sockets.Connection, config *tls.Config) (sc sockets.TCPConnection) {

	return nil
}

type handshakeFn func(ctx context.Context, handler func(cause error))

type Connection struct {
	// impl sockets tcp Conn
	inner sockets.TCPConnection
}
