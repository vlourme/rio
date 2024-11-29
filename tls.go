package rio

import (
	"context"
	"crypto/tls"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"net"
	"sync"
	"time"
)

type TLSConnection interface {
	Connection
	Handshake() (future async.Future[async.Void])
	VerifyHostname(host string) (err error)
	ConnectionState() (state tls.ConnectionState)
	OCSPResponse() (b []byte)
}

type tlsHandshakeFn func() (future async.Future[async.Void])

func newTLSConnection(inner Connection, config *tls.Config) *tlsConnection {
	return &tlsConnection{
		inner:        inner,
		config:       config,
		locker:       new(sync.Mutex),
		handshakeFn:  nil,
		state:        tls.ConnectionState{},
		ocspResponse: nil,
	}
}

type tlsConnection struct {
	inner        Connection
	config       *tls.Config
	locker       sync.Locker
	handshakeFn  tlsHandshakeFn
	state        tls.ConnectionState
	ocspResponse []byte
}

func (conn *tlsConnection) Context() (ctx context.Context) {
	ctx = conn.inner.Context()
	return
}

func (conn *tlsConnection) ConfigContext(config func(ctx context.Context) context.Context) {
	conn.inner.ConfigContext(config)
}

func (conn *tlsConnection) LocalAddr() net.Addr {
	return conn.inner.LocalAddr()
}

func (conn *tlsConnection) RemoteAddr() net.Addr {
	return conn.inner.RemoteAddr()
}

func (conn *tlsConnection) SetReadTimeout(d time.Duration) error {
	return conn.inner.SetReadTimeout(d)
}

func (conn *tlsConnection) SetWriteTimeout(d time.Duration) error {
	return conn.inner.SetWriteTimeout(d)
}

func (conn *tlsConnection) SetReadBuffer(n int) error {
	return conn.inner.SetReadBuffer(n)
}

func (conn *tlsConnection) SetWriteBuffer(n int) error {
	return conn.inner.SetWriteBuffer(n)
}

func (conn *tlsConnection) SetInboundBuffer(n int) {
	conn.inner.SetInboundBuffer(n)
}

func (conn *tlsConnection) Read() (future async.Future[transport.Inbound]) {
	//TODO implement me
	panic("implement me")
}

func (conn *tlsConnection) Write(b []byte) (future async.Future[transport.Outbound]) {
	//TODO implement me
	panic("implement me")
}

func (conn *tlsConnection) Close() (future async.Future[async.Void]) {
	future = conn.inner.Close()
	return
}

func (conn *tlsConnection) Handshake() (future async.Future[async.Void]) {
	//TODO implement me
	panic("implement me")
}

func (conn *tlsConnection) VerifyHostname(host string) (err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *tlsConnection) ConnectionState() tls.ConnectionState {
	return conn.state
}

func (conn *tlsConnection) OCSPResponse() []byte {
	return conn.ocspResponse
}
