package security

import (
	"context"
	"crypto/tls"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
)

type HandshakeResult struct {
	Cipher Cipher
}

type Handshake func(ctx context.Context, conn transport.Connection, config *tls.Config) (future async.Future[HandshakeResult])
