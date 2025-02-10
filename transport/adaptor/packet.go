package adaptor

import (
	"github.com/brickingsoft/rio/transport"
	"net"
	"time"
)

func Packet(conn transport.PacketConnection) net.PacketConn {
	return &packet{
		conn: conn,
		rch:  make(chan rwResult, 1),
		wch:  make(chan rwResult, 1),
	}
}

type packet struct {
	conn transport.PacketConnection
	rch  chan rwResult
	wch  chan rwResult
}

func (conn *packet) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *packet) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *packet) Close() error {
	return conn.conn.Close()
}

func (conn *packet) LocalAddr() net.Addr {
	return conn.conn.LocalAddr()
}

func (conn *packet) SetDeadline(t time.Time) error {
	conn.conn.SetReadTimeout(time.Until(t))
	conn.conn.SetWriteTimeout(time.Until(t))
	return nil
}

func (conn *packet) SetReadDeadline(t time.Time) error {
	conn.conn.SetReadTimeout(time.Until(t))
	return nil
}

func (conn *packet) SetWriteDeadline(t time.Time) error {
	conn.conn.SetWriteTimeout(time.Until(t))
	return nil
}
