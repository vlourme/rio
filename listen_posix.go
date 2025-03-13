//go:build !linux

package rio

import (
	"context"
	"net"
)

func (lc *ListenConfig) Listen(ctx context.Context, network string, address string) (ln net.Listener, err error) {
	config := net.ListenConfig{}
	config.KeepAlive = lc.KeepAlive
	config.KeepAliveConfig = lc.KeepAliveConfig
	config.Control = lc.Control
	config.SetMultipathTCP(lc.MultipathTCP)

	ln, err = config.Listen(ctx, network, address)
	if err != nil {
		return
	}
	switch v := ln.(type) {
	case *net.TCPListener:
		return &TCPListener{v}, nil
	case *net.UnixListener:
		return &UnixListener{v}, nil
	default:
		return ln, nil
	}
}

func (lc *ListenConfig) ListenPacket(ctx context.Context, network, address string) (c net.PacketConn, err error) {
	config := net.ListenConfig{}
	config.KeepAlive = lc.KeepAlive
	config.KeepAliveConfig = lc.KeepAliveConfig
	config.Control = lc.Control

	c, err = config.ListenPacket(ctx, network, address)
	if err != nil {
		return
	}
	switch v := c.(type) {
	case *net.UDPConn:
		return &UDPConn{v}, nil
	case *net.UnixConn:
		return &UnixConn{v}, nil
	case *net.IPConn:
		return &IPConn{v}, nil
	default:
		return c, nil
	}
}
