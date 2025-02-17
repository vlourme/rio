//go:build linux

package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"github.com/brickingsoft/rio/pkg/sys"
	"net"
	"syscall"
	"time"
)

func Listen(network string, addr string) (ln net.Listener, err error) {
	config := ListenConfig{}
	ctx := context.Background()
	ln, err = config.Listen(ctx, network, addr)
	return
}

type ListenConfig struct {
	Control         func(network, address string, c syscall.RawConn) error
	KeepAlive       time.Duration
	KeepAliveConfig net.KeepAliveConfig
	UseSendZC       bool
	MultipathTCP    bool
	FastOpen        int
	VortexesOptions []aio.Option
}

func (lc *ListenConfig) SetAIOOptions(opts ...aio.Option) {
	lc.VortexesOptions = opts
}

func (lc *ListenConfig) SetFastOpen(n int) {
	if n < 1 {
		return
	}
	if n > 999 {
		n = 256
	}
	lc.FastOpen = n
}

func (lc *ListenConfig) SetMultipathTCP(use bool) {
	lc.MultipathTCP = use
}

func (lc *ListenConfig) Listen(ctx context.Context, network string, address string) (ln net.Listener, err error) {
	addr, _, _, addrErr := sys.ResolveAddr(network, address)
	if addrErr != nil {
		err = &net.OpError{Op: "listen", Net: network, Source: nil, Addr: nil, Err: addrErr}
		return
	}
	switch a := addr.(type) {
	case *net.TCPAddr:
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
