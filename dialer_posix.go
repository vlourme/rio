//go:build !linux

package rio

import (
	"context"
	"net"
	"time"
)

var (
	DefaultDialer = Dialer{}
)

type Dialer struct {
	net.Dialer
}

func (d *Dialer) SetFastOpen(_ int) {
	return
}

func Dial(network string, address string) (net.Conn, error) {
	ctx := context.Background()
	return DialContext(ctx, network, address)
}

func DialContext(ctx context.Context, network string, address string) (net.Conn, error) {
	c, err := DefaultDialer.DialContext(ctx, network, address)
	if err != nil {
		return nil, err
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

func DialTimeout(network string, address string, timeout time.Duration) (net.Conn, error) {
	c, err := net.DialTimeout(network, address, timeout)
	if err != nil {
		return nil, err
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
