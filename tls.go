package rio

import (
	"context"
	"crypto/tls"
	"github.com/brickingsoft/rio/pkg/aio"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"net"
	"time"
)

type TLSConnectionBuilder func(conn TCPConnection, config *tls.Config) TLSConnection

type TLSConnection interface {
	TCPConnection
	ConnectionState() tls.ConnectionState
	OCSPResponse() []byte
	VerifyHostname(host string) error
	CloseWrite() (future async.Future[async.Void])
	Handshake() (future async.Future[async.Void])
}

type tlsConnection struct {
	inner       TCPConnection
	config      *tls.Config
	isClient    bool
	handshakeFn func() (future async.Future[async.Void])
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

func (conn *tlsConnection) MultipathTCP() bool {
	return conn.inner.MultipathTCP()
}

func (conn *tlsConnection) SetNoDelay(noDelay bool) error {
	return conn.inner.SetNoDelay(noDelay)
}

func (conn *tlsConnection) SetLinger(sec int) error {
	return conn.inner.SetLinger(sec)
}

func (conn *tlsConnection) SetKeepAlive(keepalive bool) error {
	return conn.inner.SetKeepAlive(keepalive)
}

func (conn *tlsConnection) SetKeepAlivePeriod(period time.Duration) error {
	return conn.inner.SetKeepAlivePeriod(period)
}

func (conn *tlsConnection) SetKeepAliveConfig(config aio.KeepAliveConfig) error {
	return conn.inner.SetKeepAliveConfig(config)
}
