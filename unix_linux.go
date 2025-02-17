//go:build linux

package rio

import (
	"context"
	"net"
)

func ListenUnix(network string, addr *net.UnixAddr) (*UnixListener, error) {
	config := ListenConfig{}
	ctx := context.Background()
	return config.ListenUnix(ctx, network, addr)
}

func (lc *ListenConfig) ListenUnix(ctx context.Context, network string, addr *net.UnixAddr) (*UnixListener, error) {
	return nil, nil
}

func ListenUnixgram(network string, addr *net.UnixAddr) (*UnixConn, error) {
	config := ListenConfig{}
	ctx := context.Background()
	return config.ListenUnixgram(ctx, network, addr)
}

func (lc *ListenConfig) ListenUnixgram(ctx context.Context, network string, addr *net.UnixAddr) (*UnixConn, error) {
	return nil, nil
}

type UnixConn struct {
	net.UnixConn
}

type UnixListener struct {
}

func (ln *UnixListener) Accept() (net.Conn, error) {
	//TODO implement me
	panic("implement me")
}

func (ln *UnixListener) Close() error {
	//TODO implement me
	panic("implement me")
}

func (ln *UnixListener) Addr() net.Addr {
	//TODO implement me
	panic("implement me")
}
