package security

import (
	"context"
	"github.com/brickingsoft/rio/pkg/aio"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"net"
	"time"
)

type TLSConnection struct {
	fd aio.NetFd
}

func (conn *TLSConnection) Context() (ctx context.Context) {
	//TODO implement me
	panic("implement me")
}

func (conn *TLSConnection) ConfigContext(config func(ctx context.Context) context.Context) {
	//TODO implement me
	panic("implement me")
}

func (conn *TLSConnection) LocalAddr() (addr net.Addr) {
	//TODO implement me
	panic("implement me")
}

func (conn *TLSConnection) RemoteAddr() (addr net.Addr) {
	//TODO implement me
	panic("implement me")
}

func (conn *TLSConnection) SetReadTimeout(d time.Duration) (err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *TLSConnection) SetWriteTimeout(d time.Duration) (err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *TLSConnection) SetReadBuffer(n int) (err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *TLSConnection) SetWriteBuffer(n int) (err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *TLSConnection) SetInboundBuffer(n int) {
	//TODO implement me
	panic("implement me")
}

func (conn *TLSConnection) Read() (future async.Future[transport.Inbound]) {
	//TODO implement me
	panic("implement me")
}

func (conn *TLSConnection) Write(b []byte) (future async.Future[transport.Outbound]) {
	//TODO implement me
	panic("implement me")
}

func (conn *TLSConnection) Close() (future async.Future[async.Void]) {
	//TODO implement me
	panic("implement me")
}

func (conn *TLSConnection) MultipathTCP() bool {
	//TODO implement me
	panic("implement me")
}

func (conn *TLSConnection) SetNoDelay(noDelay bool) (err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *TLSConnection) SetLinger(sec int) (err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *TLSConnection) SetKeepAlive(keepalive bool) (err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *TLSConnection) SetKeepAlivePeriod(period time.Duration) (err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *TLSConnection) SetKeepAliveConfig(config aio.KeepAliveConfig) (err error) {
	//TODO implement me
	panic("implement me")
}
