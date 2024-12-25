package rio

import (
	"crypto/tls"
	"github.com/brickingsoft/rxp/async"
)

func TLSServer(conn TCPConnection, config *tls.Config) TLSConnection {
	c := &tlsConnection{
		inner:  conn,
		config: config,
	}
	c.handshakeFn = c.serverHandshake
	return c
}

func (conn *tlsConnection) serverHandshake() (future async.Future[async.Void]) {

	return
}
