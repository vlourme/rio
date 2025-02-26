//go:build !linux

package rio

import (
	"net"
)

type UnixListener struct {
	*net.UnixListener
}

type UnixConn struct {
	*net.UnixConn
}

func ListenUnix(network string, addr *net.UnixAddr) (*UnixListener, error) {
	ln, err := net.ListenUnix(network, addr)
	if err != nil {
		return nil, err
	}
	return &UnixListener{ln}, nil
}

func ListenUnixgram(network string, addr *net.UnixAddr) (*UnixConn, error) {
	c, err := net.ListenUnixgram(network, addr)
	if err != nil {
		return nil, err
	}
	return &UnixConn{c}, nil
}
