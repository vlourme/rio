//go:build linux

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

func (lc *ListenConfig) ListenIP(ctx context.Context, network string, addr *net.IPAddr) (*IPConn, error) {
	return nil, nil
}

type IPConn struct {
	net.IPConn
}
