package transport

import (
	"github.com/brickingsoft/rxp/async"
	"net"
	"time"
)

func AdaptToNetConn(conn Connection) net.Conn {
	return &netConn{conn}
}

type netConn struct {
	inner Connection
}

func (conn *netConn) Connection() Connection {
	return conn.inner
}

func (conn *netConn) Read(b []byte) (n int, err error) {
	bLen := len(b)
	if bLen == 0 {
		return
	}
	af := async.AwaitableFuture(conn.inner.Read())
	in, rErr := af.Await()
	if rErr != nil {
		err = rErr
		return
	}
	n = in.Received()
	if n == 0 {
		return
	}
	n, err = in.Reader().Read(b)
	return
}

func (conn *netConn) Write(b []byte) (n int, err error) {
	if len(b) == 0 {
		return
	}
	af := async.AwaitableFuture(conn.inner.Write(b))
	n, err = af.Await()
	return
}

func (conn *netConn) Close() error {
	af := async.AwaitableFuture(conn.inner.Close())
	_, err := af.Await()
	return err
}

func (conn *netConn) LocalAddr() net.Addr {
	return conn.inner.LocalAddr()
}

func (conn *netConn) RemoteAddr() net.Addr {
	return conn.inner.RemoteAddr()
}

func (conn *netConn) SetDeadline(t time.Time) error {
	conn.inner.SetReadTimeout(time.Until(t))
	conn.inner.SetWriteTimeout(time.Until(t))
	return nil
}

func (conn *netConn) SetReadDeadline(t time.Time) error {
	conn.inner.SetReadTimeout(time.Until(t))
	return nil
}

func (conn *netConn) SetWriteDeadline(t time.Time) error {
	conn.inner.SetWriteTimeout(time.Until(t))
	return nil
}
