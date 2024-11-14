package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/sockets"
	"time"
)

type TCPConnection interface {
	Connection
	SetNoDelay(noDelay bool) (err error)
	SetLinger(sec int) (err error)
	SetKeepAlive(keepalive bool) (err error)
	SetKeepAlivePeriod(period time.Duration) (err error)
}

func newTCPConnection(ctx context.Context, inner sockets.Connection) (conn TCPConnection) {
	c := newConnection(ctx, inner)
	conn = &tcpConnection{
		connection: *c,
	}
	return
}

type tcpConnection struct {
	connection
}

func (conn *tcpConnection) SetNoDelay(noDelay bool) (err error) {
	tcp, isTCP := conn.connection.inner.(sockets.TCPConnection)
	if !isTCP {
		err = errors.New("rio: not a TCP connection")
		return
	}
	err = tcp.SetNoDelay(noDelay)
	return
}

func (conn *tcpConnection) SetLinger(sec int) (err error) {
	tcp, isTCP := conn.connection.inner.(sockets.TCPConnection)
	if !isTCP {
		err = errors.New("rio: not a TCP connection")
		return
	}
	err = tcp.SetLinger(sec)
	return
}

func (conn *tcpConnection) SetKeepAlive(keepalive bool) (err error) {
	tcp, isTCP := conn.connection.inner.(sockets.TCPConnection)
	if !isTCP {
		err = errors.New("rio: not a TCP connection")
		return
	}
	err = tcp.SetKeepAlive(keepalive)
	return
}

func (conn *tcpConnection) SetKeepAlivePeriod(period time.Duration) (err error) {
	tcp, isTCP := conn.connection.inner.(sockets.TCPConnection)
	if !isTCP {
		err = errors.New("rio: not a TCP connection")
		return
	}
	err = tcp.SetKeepAlivePeriod(period)
	return
}
