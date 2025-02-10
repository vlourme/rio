package adaptor

import (
	"context"
	"github.com/brickingsoft/errors"
	"github.com/brickingsoft/rio/transport"
	"net"
	"time"
)

func Packet(conn transport.PacketConnection) net.PacketConn {
	return &packet{
		conn: conn,
		rch:  make(chan prwResult, 1),
		wch:  make(chan prwResult, 1),
	}
}

type prwResult struct {
	n    int
	addr net.Addr
	err  error
}

type packet struct {
	conn transport.PacketConnection
	rch  chan prwResult
	wch  chan prwResult
}

func (conn *packet) ReadFrom(b []byte) (n int, addr net.Addr, err error) {
	bLen := len(b)
	if bLen == 0 {
		return
	}
	conn.conn.SetInboundBuffer(bLen)
	conn.conn.ReadFrom().OnComplete(func(ctx context.Context, in transport.PacketInbound, err error) {
		if err != nil {
			conn.rch <- prwResult{n: 0, err: err}
			return
		}
		rn := copy(b, in.Bytes)
		conn.rch <- prwResult{n: rn, addr: in.Addr, err: nil}
		return
	})
	r := <-conn.rch
	n, addr, err = r.n, r.addr, r.err
	return
}

func (conn *packet) WriteTo(b []byte, addr net.Addr) (n int, err error) {
	if bLen := len(b); bLen == 0 {
		return
	}
	if addr == nil {
		err = errors.New("addr is nil")
		return
	}
	conn.conn.WriteTo(b, addr).OnComplete(func(ctx context.Context, written int, err error) {
		conn.wch <- prwResult{n: written, err: err}
	})
	r := <-conn.wch
	n, err = r.n, r.err
	return
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
