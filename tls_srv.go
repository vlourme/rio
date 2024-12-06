package rio

import (
	"crypto/tls"
	"github.com/brickingsoft/rxp/async"
)

func serverTLS(conn Connection, config *tls.Config) (sc TLSConnection, err error) {
	c := newTLSConnection(conn, config)
	c.handshakeFn = c.serverHandshake
	return c, nil
}

func (conn *tlsConnection) serverHandshake() (future async.Future[async.Void]) {

	return
}
