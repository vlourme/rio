package rio

import (
	"crypto/tls"
	"github.com/brickingsoft/rxp/async"
)

func serverTLS(conn Connection, config *tls.Config) TLSConnection {
	c := newTLSConnection(conn, config)
	c.handshakeFn = c.serverHandshake
	return c
}

func (conn *tlsConnection) serverHandshake() (future async.Future[async.Void]) {

	return
}
