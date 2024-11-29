package rio

import (
	"crypto/tls"
	"github.com/brickingsoft/rxp/async"
)

type TLSConnection interface {
	Connection
	Handshake() (future async.Future[async.Void])
	VerifyHostname(host string) (err error)
	ConnectionState() (state tls.ConnectionState)
	OCSPResponse() (b []byte)
}

func serverTLS(conn Connection, config *tls.Config) (sc TLSConnection) {
	// todo
	return
}

func clientTLS(conn Connection, config *tls.Config) (sc TLSConnection) {
	// todo
	return
}
