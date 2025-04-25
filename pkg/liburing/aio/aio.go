//go:build linux

package aio

import (
	"context"
	"net"
	"syscall"
	"time"
)

type Control func(ctx context.Context, network string, address string, raw syscall.RawConn) error

type AsyncIO interface {
	Listen(ctx context.Context, network string, proto int, addr net.Addr, reusePort bool, control Control) (ln *Listener, err error)
	ListenPacket(ctx context.Context, network string, proto int, addr net.Addr, ifi *net.Interface, reusePort bool, control Control) (conn *Conn, err error)
	Connect(ctx context.Context, deadline time.Time, network string, proto int, laddr net.Addr, raddr net.Addr, control Control) (conn *Conn, err error)
	Close() (err error)
}
