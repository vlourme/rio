//go:build !linux

package rio

import (
	"net"
)

type TCPListener struct {
	*net.TCPListener
}

type TCPConn struct {
	*net.TCPConn
}

func ListenTCP(network string, addr *net.TCPAddr) (*TCPListener, error) {
	ln, err := net.ListenTCP(network, addr)
	if err != nil {
		return nil, err
	}
	return &TCPListener{ln}, nil
}
