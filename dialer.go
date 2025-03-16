package rio

import (
	"context"
	"net"
	"strings"
	"syscall"
	"time"
)

var (
	DefaultDialer = Dialer{
		Timeout:            15 * time.Second,
		Deadline:           time.Time{},
		LocalAddr:          nil,
		KeepAlive:          0,
		KeepAliveConfig:    net.KeepAliveConfig{Enable: true},
		MultipathTCP:       false,
		FastOpen:           false,
		QuickAck:           false,
		UseSendZC:          false,
		AutoFixedFdInstall: false,
		Control:            nil,
		ControlContext:     nil,
	}
)

func Dial(network string, address string) (net.Conn, error) {
	ctx := context.Background()
	return DialContext(ctx, network, address)
}

func DialContext(ctx context.Context, network string, address string) (net.Conn, error) {
	dialer := DefaultDialer
	if strings.HasPrefix(network, "tcp") {
		dialer.SetFastOpen(true)
		dialer.SetQuickAck(true)
	}
	return dialer.DialContext(ctx, network, address)
}

func DialTimeout(network string, address string, timeout time.Duration) (net.Conn, error) {
	ctx := context.Background()
	return DialContextTimeout(ctx, network, address, timeout)
}

func DialContextTimeout(ctx context.Context, network string, address string, timeout time.Duration) (net.Conn, error) {
	dialer := DefaultDialer
	dialer.Timeout = timeout
	if strings.HasPrefix(network, "tcp") {
		dialer.SetFastOpen(true)
		dialer.SetQuickAck(true)
	}
	return dialer.DialContext(ctx, network, address)
}

type Dialer struct {
	Timeout            time.Duration
	Deadline           time.Time
	KeepAlive          time.Duration
	KeepAliveConfig    net.KeepAliveConfig
	LocalAddr          net.Addr
	MultipathTCP       bool
	FastOpen           bool
	QuickAck           bool
	UseSendZC          bool
	AutoFixedFdInstall bool
	Control            func(network, address string, c syscall.RawConn) error
	ControlContext     func(ctx context.Context, network, address string, c syscall.RawConn) error
}

func (d *Dialer) SetFastOpen(use bool) {
	d.FastOpen = use
}

func (d *Dialer) SetQuickAck(use bool) {
	d.QuickAck = use
}

func (d *Dialer) SetMultipathTCP(use bool) {
	d.MultipathTCP = use
}

func (d *Dialer) SetUseSendZC(use bool) {
	d.UseSendZC = use
}

func (d *Dialer) SetAutoFixedFdInstall(auto bool) {
	d.AutoFixedFdInstall = auto
}

func (d *Dialer) deadline(ctx context.Context, now time.Time) (earliest time.Time) {
	if d.Timeout != 0 {
		earliest = now.Add(d.Timeout)
	}
	if deadline, ok := ctx.Deadline(); ok {
		earliest = minNonzeroTime(earliest, deadline)
	}
	return minNonzeroTime(earliest, d.Deadline)
}

func minNonzeroTime(a, b time.Time) time.Time {
	if a.IsZero() {
		return b
	}
	if b.IsZero() || a.Before(b) {
		return a
	}
	return b
}
