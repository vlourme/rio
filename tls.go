package rio

import (
	"context"
	"crypto/tls"
	"github.com/brickingsoft/rio/security"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"net"
	"sync/atomic"
	"time"
)

type TLSConnectionBuilder func(conn Connection, config *tls.Config) TLSConnection

type TLSConnection interface {
	Connection
	ConnectionState() tls.ConnectionState
	OCSPResponse() []byte
	VerifyHostname(host string) error
	CloseWrite() (future async.Future[async.Void])
	Handshake() (future async.Future[async.Void])
}

func TLSClient(conn Connection, config *tls.Config) TLSConnection {
	c := &tlsConnection{
		inner:    conn,
		config:   config,
		isClient: true,
	}
	c.handshake = security.ClientHandShaker(conn, config)
	return c
}

func TLSServer(conn Connection, config *tls.Config) TLSConnection {
	c := &tlsConnection{
		inner:  conn,
		config: config,
	}
	c.handshake = security.ServerHandShaker(conn, config)
	return c
}

type tlsConnection struct {
	inner             Connection
	config            *tls.Config
	isClient          bool
	handshake         security.HandShaker
	handshakeComplete atomic.Bool
	handshakeErr      error
}

func (conn *tlsConnection) Read() (future async.Future[transport.Inbound]) {
	//TODO implement me
	panic("implement me")
}

func (conn *tlsConnection) Write(b []byte) (future async.Future[int]) {
	//TODO implement me
	panic("implement me")
}

func (conn *tlsConnection) Close() (future async.Future[async.Void]) {
	//TODO implement me
	panic("implement me")
}

func (conn *tlsConnection) Sendfile(file string) (future async.Future[int]) {
	//TODO implement me
	panic("implement me")
}

func (conn *tlsConnection) ConnectionState() tls.ConnectionState {
	//TODO implement me
	panic("implement me")
}

func (conn *tlsConnection) OCSPResponse() []byte {
	//TODO implement me
	panic("implement me")
}

func (conn *tlsConnection) VerifyHostname(host string) error {
	//TODO implement me
	panic("implement me")
}

func (conn *tlsConnection) CloseWrite() (future async.Future[async.Void]) {
	//TODO implement me
	panic("implement me")
}

func (conn *tlsConnection) Handshake() (future async.Future[async.Void]) {
	//TODO implement me
	panic("implement me")
}

func (conn *tlsConnection) Context() context.Context {
	return conn.inner.Context()
}

func (conn *tlsConnection) ConfigContext(config func(ctx context.Context) context.Context) {
	conn.inner.ConfigContext(config)
}

func (conn *tlsConnection) Fd() int {
	return conn.inner.Fd()
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
