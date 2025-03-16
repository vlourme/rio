//go:build !linux

package rio

import (
	"net"
)

// TCPListener is a TCP network listener. Clients should typically
// use variables of type [net.Listener] instead of assuming TCP.
type TCPListener struct {
	*net.TCPListener
}

// TCPConn is an implementation of the [net.Conn] interface for TCP network
// connections.
type TCPConn struct {
	*net.TCPConn
}

// ListenTCP acts like [Listen] for TCP networks.
//
// The network must be a TCP network name; see func Dial for details.
//
// If the IP field of laddr is nil or an unspecified IP address,
// ListenTCP listens on all available unicast and anycast IP addresses
// of the local system.
// If the Port field of laddr is 0, a port number is automatically
// chosen.
func ListenTCP(network string, addr *net.TCPAddr) (*TCPListener, error) {
	ln, err := net.ListenTCP(network, addr)
	if err != nil {
		return nil, err
	}
	return &TCPListener{ln}, nil
}
