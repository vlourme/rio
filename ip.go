package rio

import (
	"context"
	"net"
)

// ListenIP acts like [ListenPacket] for IP networks.
//
// The network must be an IP network name; see func Dial for details.
//
// If the IP field of laddr is nil or an unspecified IP address,
// ListenIP listens on all available IP addresses of the local system
// except multicast IP addresses.
func ListenIP(network string, addr *net.IPAddr) (*IPConn, error) {
	config := ListenConfig{}
	ctx := context.Background()
	return config.ListenIP(ctx, network, addr)
}

// ListenIP acts like [ListenPacket] for IP networks.
//
// The network must be an IP network name; see func Dial for details.
//
// If the IP field of laddr is nil or an unspecified IP address,
// ListenIP listens on all available IP addresses of the local system
// except multicast IP addresses.
func (lc *ListenConfig) ListenIP(_ context.Context, network string, addr *net.IPAddr) (*IPConn, error) {
	c, err := net.ListenIP(network, addr)
	if err != nil {
		return nil, err
	}
	return &IPConn{c}, nil
}

// IPConn is the implementation of the [net.Conn] and [net.PacketConn] interfaces
// for IP network connections.
type IPConn struct {
	*net.IPConn
}
