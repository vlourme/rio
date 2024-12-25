package security

import (
	"context"
	"crypto/tls"
	"github.com/brickingsoft/rio/pkg/aio"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
)

func ServerHandshake(ctx context.Context, conn transport.Transport, config *tls.Config) (future async.Future[HandshakeResult]) {

	return
}

func Server(ctx context.Context, fd aio.NetFd, config *Config) Connection {
	c := &TLSConnection{
		ctx:      ctx,
		fd:       fd,
		rb:       transport.NewInboundBuffer(),
		rbs:      defaultReadBufferSize,
		isClient: false,
		config:   config,
	}
	c.handshakeFn = c.serverHandshake
	return c
}

type serverHandshakeState struct {
	c            *TLSConnection
	ctx          context.Context
	clientHello  *clientHelloMsg
	hello        *serverHelloMsg
	suite        *cipherSuite
	ecdheOk      bool
	ecSignOk     bool
	rsaDecryptOk bool
	rsaSignOk    bool
	sessionState *SessionState
	finishedHash finishedHash
	masterSecret []byte
	cert         *Certificate
}

func (conn *TLSConnection) serverHandshake() (future async.Future[async.Void]) {

	return
}

func (conn *TLSConnection) readClientHello() (future async.Future[*clientHelloMsg]) {

	return
}
