//go:build linux

package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/sys"
	"net"
)

func (lc *ListenConfig) Listen(ctx context.Context, network string, address string) (ln net.Listener, err error) {
	addr, _, _, addrErr := sys.ResolveAddr(network, address)
	if addrErr != nil {
		err = &net.OpError{Op: "listen", Net: network, Source: nil, Addr: nil, Err: addrErr}
		return
	}
	switch a := addr.(type) {
	case *net.TCPAddr:
		lc.SetFastOpen(true)
		lc.SetQuickAck(true)
		lc.SetReusePort(true)
		ln, err = lc.ListenTCP(ctx, network, a)
		break
	case *net.UnixAddr:
		ln, err = lc.ListenUnix(ctx, network, a)
		break
	default:
		err = &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: &net.AddrError{Err: "unexpected address type", Addr: address}}
		break
	}
	return
}

func (lc *ListenConfig) ListenPacket(ctx context.Context, network, address string) (c net.PacketConn, err error) {
	addr, _, _, addrErr := sys.ResolveAddr(network, address)
	if addrErr != nil {
		err = &net.OpError{Op: "listen", Net: network, Source: nil, Addr: nil, Err: addrErr}
		return
	}

	switch a := addr.(type) {
	case *net.UDPAddr:
		c, err = lc.ListenUDP(ctx, network, a)
		break
	case *net.IPAddr:
		c, err = lc.ListenIP(ctx, network, a)
		break
	case *net.UnixAddr:
		c, err = lc.ListenUnixgram(ctx, network, a)
		break
	default:
		err = &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: &net.AddrError{Err: "unexpected address type", Addr: address}}
		break
	}
	return
}
