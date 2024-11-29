package rio

import (
	"crypto/tls"
	"github.com/brickingsoft/rxp/async"
)

func clientTLS(conn Connection, config *tls.Config) TLSConnection {
	c := newTLSConnection(conn, config)
	c.handshakeFn = c.clientHandshake
	return c
}

func (conn *tlsConnection) clientHandshake() (future async.Future[async.Void]) {

	return
}
