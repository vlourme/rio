package security

import (
	"context"
	"crypto/tls"
	"github.com/brickingsoft/rio/pkg/sockets"
)

func Serve(ctx context.Context, conn sockets.Connection, config *tls.Config) (sc sockets.Connection) {

	sc = &Connection{
		Connection:  conn,
		ctx:         ctx,
		config:      config,
		handshakeFn: nil,
	}
	return
}

func Client(ctx context.Context, conn sockets.Connection, config *tls.Config) (sc sockets.Connection) {

	return nil
}

type HandshakeFn func(ctx context.Context, handler func(cause error))

type Connection struct {
	sockets.Connection
	ctx         context.Context
	config      *tls.Config
	handshakeFn HandshakeFn
}

func (conn *Connection) Read(p []byte, handler sockets.ReadHandler) {
	//TODO implement me
	panic("implement me")
}

func (conn *Connection) Write(p []byte, handler sockets.WriteHandler) {
	//TODO implement me
	panic("implement me")
}

func (conn *Connection) Close() (err error) {
	//TODO implement me
	panic("implement me")
}
