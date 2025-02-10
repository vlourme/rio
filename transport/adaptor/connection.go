package adaptor

import (
	"context"
	"github.com/brickingsoft/rio/transport"
	"net"
	"time"
)

func Connection(conn transport.Connection) net.Conn {
	return &connection{
		conn: conn,
		rch:  make(chan crwResult, 1),
		wch:  make(chan crwResult, 1),
	}
}

type crwResult struct {
	n   int
	err error
}

type connection struct {
	conn transport.Connection
	rch  chan crwResult
	wch  chan crwResult
}

func (conn *connection) Read(b []byte) (n int, err error) {
	if bLen := len(b); bLen == 0 {
		return
	}
	conn.conn.Read().OnComplete(func(ctx context.Context, in transport.Inbound, err error) {
		if err != nil {
			conn.rch <- crwResult{n: 0, err: err}
			return
		}
		rn, rErr := in.Read(b)
		conn.rch <- crwResult{n: rn, err: rErr}
		return
	})
	r := <-conn.rch
	n, err = r.n, r.err
	return
}

func (conn *connection) Write(b []byte) (n int, err error) {
	if bLen := len(b); bLen == 0 {
		return
	}
	conn.conn.Write(b).OnComplete(func(ctx context.Context, written int, err error) {
		conn.wch <- crwResult{n: written, err: err}
	})
	r := <-conn.wch
	n, err = r.n, r.err
	return
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
