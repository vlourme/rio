//go:build linux

package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/sys"
	"net"
)

func ListenPacket(network string, addr string) (ln net.PacketConn, err error) {
	config := ListenConfig{}
	ctx := context.Background()
	ln, err = config.ListenPacket(ctx, network, addr)
	return
}

func (lc *ListenConfig) ListenPacket(ctx context.Context, network, address string) (conn net.PacketConn, err error) {
	addr, _, _, addrErr := sys.ResolveAddr(network, address)
	if addrErr != nil {
		err = &net.OpError{Op: "listen", Net: network, Source: nil, Addr: nil, Err: addrErr}
		return
	}
	switch a := addr.(type) {
	case *net.UDPAddr:
		conn, err = lc.ListenUDP(ctx, network, a)
		break
	case *net.IPAddr:
		// todo
		//conn, err = lc.ListenIP(ctx, network, a)
		break
	case *net.UnixAddr:
		// todo
		//conn, err = lc.ListenUnixgram(ctx, network, a)
		break
	default:
		err = &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: &net.AddrError{Err: "unexpected address type", Addr: address}}
		break
	}
	return
}
