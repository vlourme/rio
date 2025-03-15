package rio

import (
	"context"
	"net"
	"syscall"
	"time"
)

func Listen(network string, addr string) (ln net.Listener, err error) {
	config := ListenConfig{
		Control:            nil,
		KeepAlive:          0,
		KeepAliveConfig:    net.KeepAliveConfig{},
		MultipathTCP:       false,
		FastOpen:           false,
		QuickAck:           false,
		ReusePort:          false,
		UseSendZC:          false,
		MultishotAccept:    false,
		AutoFixedFdInstall: false,
	}
	ctx := context.Background()
	ln, err = config.Listen(ctx, network, addr)
	return
}

func ListenPacket(network string, addr string) (c net.PacketConn, err error) {
	config := ListenConfig{}
	ctx := context.Background()
	c, err = config.ListenPacket(ctx, network, addr)
	return
}

type ListenConfig struct {
	Control            func(network, address string, c syscall.RawConn) error
	KeepAlive          time.Duration
	KeepAliveConfig    net.KeepAliveConfig
	MultipathTCP       bool
	FastOpen           bool
	QuickAck           bool
	ReusePort          bool
	UseSendZC          bool
	MultishotAccept    bool
	AutoFixedFdInstall bool
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

func (lc *ListenConfig) SetUseSendZC(use bool) {
	lc.UseSendZC = use
}

func (lc *ListenConfig) SetMultishotAccept(multi bool) {
	lc.MultishotAccept = multi
}

func (lc *ListenConfig) SetAutoFixedFdInstall(auto bool) {
	lc.AutoFixedFdInstall = auto
}
