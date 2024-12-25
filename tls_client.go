package rio

import (
	"crypto/tls"
	"github.com/brickingsoft/rxp/async"
)

func TLSClient(conn TCPConnection, config *tls.Config) TLSConnection {
	c := &tlsConnection{
		inner:    conn,
		config:   config,
		isClient: true,
	}
	c.handshakeFn = c.clientHandshake
	return c
}

func (conn *tlsConnection) clientHandshake() (future async.Future[async.Void]) {

	return
}
