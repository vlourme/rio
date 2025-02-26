//go:build !linux

package rio

import (
	"net"
)

type UDPConn struct {
	*net.UDPConn
}

func ListenUDP(network string, addr *net.UDPAddr) (*UDPConn, error) {
	c, err := net.ListenUDP(network, addr)
	if err != nil {
		return nil, err
	}
	return &UDPConn{c}, nil
}

func ListenMulticastUDP(network string, ifi *net.Interface, addr *net.UDPAddr) (*UDPConn, error) {
	c, err := net.ListenMulticastUDP(network, ifi, addr)
	if err != nil {
		return nil, err
	}
	return &UDPConn{c}, nil
}
