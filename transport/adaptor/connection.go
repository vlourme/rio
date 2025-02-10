package adaptor

import (
	"github.com/brickingsoft/rio/transport"
	"net"
	"time"
)

func Connection(conn transport.Connection) net.Conn {
	return &connection{
		conn: conn,
		rch:  make(chan rwResult, 1),
		wch:  make(chan rwResult, 1),
	}
}

type rwResult struct {
	n    int
	addr net.Addr
	err  error
}

type connection struct {
	conn transport.Connection
	rch  chan rwResult
	wch  chan rwResult
}

func (conn *connection) Read(b []byte) (n int, err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *connection) Write(b []byte) (n int, err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *connection) Close() error {
	return conn.conn.Close()
}

func (conn *connection) LocalAddr() net.Addr {
	return conn.conn.LocalAddr()
}

func (conn *connection) RemoteAddr() net.Addr {
	return conn.conn.RemoteAddr()
}

func (conn *connection) SetDeadline(t time.Time) error {
	conn.conn.SetReadTimeout(time.Until(t))
	conn.conn.SetWriteTimeout(time.Until(t))
	return nil
}

func (conn *connection) SetReadDeadline(t time.Time) error {
	conn.conn.SetReadTimeout(time.Until(t))
	return nil
}

func (conn *connection) SetWriteDeadline(t time.Time) error {
	conn.conn.SetWriteTimeout(time.Until(t))
	return nil
}
