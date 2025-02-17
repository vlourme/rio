//go:build linux

package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"github.com/brickingsoft/rio/pkg/sys"
	"net"
	"runtime"
	"sync"
	"time"
)

func Dial(network string, address string) (net.Conn, error) {
	ctx := context.Background()
	return DialContext(ctx, network, address)
}

func DialContext(ctx context.Context, network string, address string) (net.Conn, error) {
	return DefaultDialer().Dial(ctx, network, address)
}

var (
	defaultDialer     = &Dialer{}
	defaultDialerOnce = sync.Once{}
)

func DefaultDialer() *Dialer {
	defaultDialerOnce.Do(func() {
		vortexes, vortexesErr := aio.New(aio.WithEntries(iouring.DefaultEntries))
		if vortexesErr != nil {
			panic(vortexesErr)
		}
		vortexes.Start(context.Background())

		defaultDialer = &Dialer{}
		defaultDialer.Timeout = 15 * time.Second
		defaultDialer.SetFastOpen(256)
		defaultDialer.SetVortexes(vortexes)

		runtime.SetFinalizer(defaultDialer, func(d *Dialer) {
			_ = d.vortexes.Close()
		})

	})
	return defaultDialer
}

type Dialer struct {
	Timeout         time.Duration
	Deadline        time.Time
	KeepAlive       time.Duration
	KeepAliveConfig net.KeepAliveConfig
	MultipathTCP    bool
	FastOpen        int
	vortexes        *aio.Vortexes
}

func (d *Dialer) SetFastOpen(n int) {
	if n < 1 {
		return
	}
	if n > 999 {
		n = 256
	}
	d.FastOpen = n
}

func (d *Dialer) SetMultipathTCP(use bool) {
	d.MultipathTCP = use
}

func (d *Dialer) SetVortexes(v *aio.Vortexes) {
	d.vortexes = v
	return
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

func (d *Dialer) Dial(ctx context.Context, network string, address string) (conn net.Conn, err error) {
	addr, _, _, addrErr := sys.ResolveAddr(network, address)
	if addrErr != nil {
		err = &net.OpError{Op: "dial", Net: network, Source: nil, Addr: nil, Err: addrErr}
		return
	}
	switch a := addr.(type) {
	case *net.TCPAddr:
		conn, err = d.DialTCP(ctx, network, nil, a)
		break
	case *net.UDPAddr:
		conn, err = d.DialUDP(ctx, network, nil, a)
		break
	case *net.UnixAddr:
		conn, err = d.DialUnix(ctx, network, nil, a)
		break
	case *net.IPAddr:
		conn, err = d.DialIP(ctx, network, nil, a)
		break
	default:
		err = &net.OpError{Op: "dial", Net: network, Source: nil, Addr: addr, Err: &net.AddrError{Err: "unexpected address type", Addr: address}}
		break
	}
	return
}

func DialTCP(network string, laddr, raddr *net.TCPAddr) (*TCPConn, error) {
	ctx := context.Background()
	return DefaultDialer().DialTCP(ctx, network, laddr, raddr)
}

func (d *Dialer) DialTCP(ctx context.Context, network string, laddr, raddr *net.TCPAddr) (*TCPConn, error) {
	// todo timeout and keep alive
	return nil, nil
}

func DialUDP(network string, laddr, raddr *net.UDPAddr) (*UDPConn, error) {
	ctx := context.Background()
	return DefaultDialer().DialUDP(ctx, network, laddr, raddr)
}

func (d *Dialer) DialUDP(ctx context.Context, network string, laddr, raddr *net.UDPAddr) (*UDPConn, error) {
	return nil, nil
}

func DialUnix(network string, laddr, raddr *net.UnixAddr) (*UnixConn, error) {
	ctx := context.Background()
	return DefaultDialer().DialUnix(ctx, network, laddr, raddr)
}

func (d *Dialer) DialUnix(ctx context.Context, network string, laddr, raddr *net.UnixAddr) (*UnixConn, error) {
	return nil, nil
}

func DialIP(network string, laddr, raddr *net.IPAddr) (*IPConn, error) {
	ctx := context.Background()
	return DefaultDialer().DialIP(ctx, network, laddr, raddr)
}

func (d *Dialer) DialIP(ctx context.Context, network string, laddr, raddr *net.IPAddr) (*IPConn, error) {
	return nil, nil
}
