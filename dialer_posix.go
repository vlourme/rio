//go:build !linux

package rio

import (
	"context"
	"net"
)

func (d *Dialer) DialContext(ctx context.Context, network, address string) (c net.Conn, err error) {
	nd := net.Dialer{
		Timeout:         d.Timeout,
		Deadline:        d.Deadline,
		LocalAddr:       d.LocalAddr,
		DualStack:       false,
		FallbackDelay:   0,
		KeepAlive:       d.KeepAlive,
		KeepAliveConfig: d.KeepAliveConfig,
		Resolver:        nil,
		Cancel:          nil,
		Control:         d.Control,
		ControlContext:  d.ControlContext,
	}
	nd.SetMultipathTCP(d.MultipathTCP)

	c, err = nd.DialContext(ctx, network, address)
	if err != nil {
		return
	}

	switch v := c.(type) {
	case *net.TCPConn:
		return &TCPConn{v}, nil
	case *net.UnixConn:
		return &UnixConn{v}, nil
	case *net.UDPConn:
		return &UDPConn{v}, nil
	case *net.IPConn:
		return &IPConn{v}, nil
	default:
		return c, nil
	}

}

func (d *Dialer) Dial(network string, address string) (c net.Conn, err error) {
	ctx := context.Background()
	return d.DialContext(ctx, network, address)
}

func DialTCP(network string, laddr, raddr *net.TCPAddr) (*TCPConn, error) {
	c, err := net.DialTCP(network, laddr, raddr)
	if err != nil {
		return nil, err
	}
	return &TCPConn{c}, nil
}

func DialUDP(network string, laddr, raddr *net.UDPAddr) (*UDPConn, error) {
	c, err := net.DialUDP(network, laddr, raddr)
	if err != nil {
		return nil, err
	}
	return &UDPConn{c}, nil
}

func DialUnix(network string, laddr, raddr *net.UnixAddr) (*UnixConn, error) {
	c, err := net.DialUnix(network, laddr, raddr)
	if err != nil {
		return nil, err
	}
	return &UnixConn{c}, nil
}

func DialIP(network string, laddr, raddr *net.IPAddr) (*IPConn, error) {
	c, err := net.DialIP(network, laddr, raddr)
	if err != nil {
		return nil, err
	}
	return &IPConn{c}, nil
}
