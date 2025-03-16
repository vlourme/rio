//go:build !linux

package rio

import (
	"context"
	"net"
)

// DialContext connects to the address on the named network using
// the provided context.
//
// The provided Context must be non-nil. If the context expires before
// the connection is complete, an error is returned. Once successfully
// connected, any expiration of the context will not affect the
// connection.
//
// When using TCP, and the host in the address parameter resolves to multiple
// network addresses, any dial timeout (from d.Timeout or ctx) is spread
// over each consecutive dial, such that each is given an appropriate
// fraction of the time to connect.
// For example, if a host has 4 IP addresses and the timeout is 1 minute,
// the connect to each single address will be given 15 seconds to complete
// before trying the next one.
//
// See func [Dial] for a description of the network and address
// parameters.
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

// Dial connects to the address on the named network.
//
// See func Dial for a description of the network and address
// parameters.
//
// Dial uses [context.Background] internally; to specify the context, use
// [Dialer.DialContext].
func (d *Dialer) Dial(network string, address string) (c net.Conn, err error) {
	ctx := context.Background()
	return d.DialContext(ctx, network, address)
}

// DialTCP acts like [Dial] for TCP networks.
//
// The network must be a TCP network name; see func Dial for details.
//
// If laddr is nil, a local address is automatically chosen.
// If the IP field of raddr is nil or an unspecified IP address, the
// local system is assumed.
func DialTCP(network string, laddr, raddr *net.TCPAddr) (*TCPConn, error) {
	c, err := net.DialTCP(network, laddr, raddr)
	if err != nil {
		return nil, err
	}
	return &TCPConn{c}, nil
}

// DialUDP acts like [Dial] for UDP networks.
//
// The network must be a UDP network name; see func [Dial] for details.
//
// If laddr is nil, a local address is automatically chosen.
// If the IP field of raddr is nil or an unspecified IP address, the
// local system is assumed.
func DialUDP(network string, laddr, raddr *net.UDPAddr) (*UDPConn, error) {
	c, err := net.DialUDP(network, laddr, raddr)
	if err != nil {
		return nil, err
	}
	return &UDPConn{c}, nil
}

// DialUnix acts like [Dial] for Unix networks.
//
// The network must be a Unix network name; see func Dial for details.
//
// If laddr is non-nil, it is used as the local address for the
// connection.
func DialUnix(network string, laddr, raddr *net.UnixAddr) (*UnixConn, error) {
	c, err := net.DialUnix(network, laddr, raddr)
	if err != nil {
		return nil, err
	}
	return &UnixConn{c}, nil
}

// DialIP acts like [Dial] for IP networks.
//
// The network must be an IP network name; see func Dial for details.
//
// If laddr is nil, a local address is automatically chosen.
// If the IP field of raddr is nil or an unspecified IP address, the
// local system is assumed.
func DialIP(network string, laddr, raddr *net.IPAddr) (*IPConn, error) {
	c, err := net.DialIP(network, laddr, raddr)
	if err != nil {
		return nil, err
	}
	return &IPConn{c}, nil
}
