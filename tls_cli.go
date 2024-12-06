package rio

import (
	"crypto/tls"
	"github.com/brickingsoft/rxp/async"
)

func clientTLS(conn Connection, config *tls.Config) (TLSConnection, error) {
	c := newTLSConnection(conn, config)
	c.handshakeFn = c.clientHandshake
	return c, nil
}

func (conn *tlsConnection) clientHandshake() (future async.Future[async.Void]) {

	return
}
