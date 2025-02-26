//go:build !linux

package rio

import (
	"net"
)

type IPConn struct {
	*net.IPConn
}

func ListenIP(network string, addr *net.IPAddr) (*IPConn, error) {
	c, err := net.ListenIP(network, addr)
	if err != nil {
		return nil, err
	}
	return &IPConn{c}, nil
}
