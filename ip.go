package rio

import (
	"context"
	"net"
)

func ListenIP(network string, addr *net.IPAddr) (*IPConn, error) {
	config := ListenConfig{}
	ctx := context.Background()
	return config.ListenIP(ctx, network, addr)
}

func (lc *ListenConfig) ListenIP(_ context.Context, network string, addr *net.IPAddr) (*IPConn, error) {
	c, err := net.ListenIP(network, addr)
	if err != nil {
		return nil, err
	}
	return &IPConn{c}, nil
}

type IPConn struct {
	*net.IPConn
}
