//go:build linux

package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/sys"
	"net"
	"strings"
	"syscall"
	"time"
)

func Listen(network string, addr string) (ln net.Listener, err error) {
	config := ListenConfig{
		Control:         nil,
		KeepAlive:       0,
		KeepAliveConfig: net.KeepAliveConfig{},
		UseSendZC:       false,
		MultipathTCP:    false,
		FastOpen:        false,
		QuickAck:        false,
		ReusePort:       false,
	}
	if strings.HasPrefix(network, "tcp") {
		config.SetFastOpen(true)
		config.SetQuickAck(true)
		config.SetReusePort(true)
	}
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
	FastOpen        bool
	QuickAck        bool
	ReusePort       bool
}

func (lc *ListenConfig) SetSendZC(use bool) {
	lc.UseSendZC = use
	return
}

func (lc *ListenConfig) SetFastOpen(use bool) {
	lc.FastOpen = use
	return
}

func (lc *ListenConfig) SetMultipathTCP(use bool) {
	lc.MultipathTCP = use
}

func (lc *ListenConfig) SetQuickAck(use bool) {
	lc.QuickAck = use
}

func (lc *ListenConfig) SetReusePort(use bool) {
	lc.ReusePort = use
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
