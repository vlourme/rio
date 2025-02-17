//go:build linux

package rio

import (
	"context"
	"net"
)

func ListenUDP(network string, addr *net.UDPAddr) (*UDPConn, error) {
	config := ListenConfig{}
	ctx := context.Background()
	return config.ListenUDP(ctx, network, addr)
}

func (lc *ListenConfig) ListenUDP(ctx context.Context, network string, addr *net.UDPAddr) (*UDPConn, error) {

	return nil, nil
}

func ListenMulticastUDP(network string, ifi *net.Interface, addr *net.UDPAddr) (*UDPConn, error) {
	config := ListenConfig{}
	ctx := context.Background()
	return config.ListenMulticastUDP(ctx, network, ifi, addr)
}

func (lc *ListenConfig) ListenMulticastUDP(ctx context.Context, network string, ifi *net.Interface, addr *net.UDPAddr) (*UDPConn, error) {
	return nil, nil
}

type UDPConn struct {
	net.UDPConn
}
