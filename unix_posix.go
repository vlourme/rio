//go:build !linux

package rio

import (
	"net"
)

// UnixListener is a Unix domain socket listener. Clients should
// typically use variables of type [net.Listener] instead of assuming Unix
// domain sockets.
type UnixListener struct {
	*net.UnixListener
}

// UnixConn is an implementation of the [net.Conn] interface for connections
// to Unix domain sockets.
type UnixConn struct {
	*net.UnixConn
}

// ListenUnix acts like [Listen] for Unix networks.
//
// The network must be "unix" or "unixpacket".
func ListenUnix(network string, addr *net.UnixAddr) (*UnixListener, error) {
	ln, err := net.ListenUnix(network, addr)
	if err != nil {
		return nil, err
	}
	return &UnixListener{ln}, nil
}

// ListenUnixgram acts like [ListenPacket] for Unix networks.
//
// The network must be "unixgram".
func ListenUnixgram(network string, addr *net.UnixAddr) (*UnixConn, error) {
	c, err := net.ListenUnixgram(network, addr)
	if err != nil {
		return nil, err
	}
	return &UnixConn{c}, nil
}
