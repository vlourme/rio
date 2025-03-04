//go:build linux

package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"github.com/brickingsoft/rio/pkg/sys"
	"net"
	"reflect"
	"syscall"
	"time"
)

var (
	DefaultDialer = Dialer{
		Timeout:         15 * time.Second,
		Deadline:        time.Time{},
		KeepAlive:       0,
		KeepAliveConfig: net.KeepAliveConfig{Enable: true},
		MultipathTCP:    false,
		FastOpen:        false,
		QuickAck:        false,
		UseSendZC:       false,
		Control:         nil,
		ControlContext:  nil,
	}
)

func Dial(network string, address string) (net.Conn, error) {
	ctx := context.Background()
	return DialContext(ctx, network, address)
}

func DialContext(ctx context.Context, network string, address string) (net.Conn, error) {
	return DefaultDialer.DialContext(ctx, network, address)
}

func DialTimeout(network string, address string, timeout time.Duration) (net.Conn, error) {
	ctx := context.Background()
	dialer := DefaultDialer
	dialer.Timeout = timeout
	return dialer.DialContext(ctx, network, address)
}

type Dialer struct {
	Timeout         time.Duration
	Deadline        time.Time
	KeepAlive       time.Duration
	KeepAliveConfig net.KeepAliveConfig
	MultipathTCP    bool
	FastOpen        bool
	QuickAck        bool
	UseSendZC       bool
	Control         func(network, address string, c syscall.RawConn) error
	ControlContext  func(ctx context.Context, network, address string, c syscall.RawConn) error
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

func (d *Dialer) DialContext(ctx context.Context, network, address string) (c net.Conn, err error) {
	addr, _, _, addrErr := sys.ResolveAddr(network, address)
	if addrErr != nil {
		err = &net.OpError{Op: "dial", Net: network, Source: nil, Addr: nil, Err: addrErr}
		return
	}
	switch a := addr.(type) {
	case *net.TCPAddr:
		c, err = d.DialTCP(ctx, network, nil, a)
		break
	case *net.UDPAddr:
		c, err = d.DialUDP(ctx, network, nil, a)
		break
	case *net.UnixAddr:
		c, err = d.DialUnix(ctx, network, nil, a)
		break
	case *net.IPAddr:
		c, err = d.DialIP(ctx, network, nil, a)
		break
	default:
		err = &net.OpError{Op: "dial", Net: network, Source: nil, Addr: addr, Err: &net.AddrError{Err: "unexpected address type", Addr: address}}
		break
	}
	return
}

func (d *Dialer) Dial(network string, address string) (c net.Conn, err error) {
	ctx := context.Background()
	return d.DialContext(ctx, network, address)
}

func DialTCP(network string, laddr, raddr *net.TCPAddr) (*TCPConn, error) {
	ctx := context.Background()
	return DefaultDialer.DialTCP(ctx, network, laddr, raddr)
}

func (d *Dialer) DialTCP(ctx context.Context, network string, laddr, raddr *net.TCPAddr) (*TCPConn, error) {
	// network
	switch network {
	case "tcp", "tcp4", "tcp6":
	default:
		return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: net.UnknownNetworkError(network)}
	}
	if raddr == nil {
		return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: errors.New("missing address")}
	}
	// vortex
	vortex, vortexErr := aio.Acquire()
	if vortexErr != nil {
		return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: vortexErr}
	}
	// fd
	now := time.Now()
	deadline := d.deadline(ctx, time.Now())
	if deadline.Before(now) {
		_ = aio.Release(vortex)
		return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: aio.Timeout}
	}

	var control ctrlCtxFn = d.ControlContext
	if control == nil && d.Control != nil {
		control = func(ctx context.Context, network string, address string, raw syscall.RawConn) error {
			return d.Control(network, address, raw)
		}
	}

	proto := syscall.IPPROTO_TCP
	if d.MultipathTCP {
		if mp, ok := sys.TryGetMultipathTCPProto(); ok {
			proto = mp
		}
	}

	fd, fdErr := newDialerFd(ctx, network, laddr, raddr, syscall.SOCK_STREAM, proto, d.FastOpen, control)
	if fdErr != nil {
		_ = aio.Release(vortex)
		return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: fdErr}
	}

	// connect
	sa, saErr := sys.AddrToSockaddr(raddr)
	if saErr != nil {
		_ = fd.Close()
		_ = aio.Release(vortex)
		return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: saErr}
	}
	rsa, rsaLen, rsaErr := sys.SockaddrToRawSockaddrAny(sa)
	if rsaErr != nil {
		_ = fd.Close()
		_ = aio.Release(vortex)
		return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: rsaErr}
	}
	future := vortex.PrepareConnect(fd.Socket(), rsa, int(rsaLen), deadline)
	_, err := future.Await(ctx)
	if err != nil {
		_ = fd.Close()
		_ = aio.Release(vortex)
		return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: err}
	}

	// local addr
	if laddr != nil {
		fd.SetLocalAddr(laddr)
	} else {
		if laddrErr := fd.LoadLocalAddr(); laddrErr != nil {
			_ = fd.Close()
			_ = aio.Release(vortex)
			return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: laddrErr}
		}
	}
	// remote addr
	if raddrErr := fd.LoadRemoteAddr(); raddrErr != nil {
		fd.SetRemoteAddr(raddr)
	}

	// conn
	useSendZC := d.UseSendZC
	if useSendZC {
		useSendZC = aio.CheckSendZCEnable()
	}

	c := &TCPConn{
		conn{
			ctx:           ctx,
			fd:            fd,
			vortex:        vortex,
			readDeadline:  time.Time{},
			writeDeadline: time.Time{},
			useZC:         useSendZC,
			pinned:        true,
		},
	}
	_ = c.SetNoDelay(true)
	// keepalive
	keepAliveConfig := d.KeepAliveConfig
	if !keepAliveConfig.Enable && d.KeepAlive >= 0 {
		keepAliveConfig = net.KeepAliveConfig{
			Enable: true,
			Idle:   d.KeepAlive,
		}
	}
	if keepAliveConfig.Enable {
		_ = c.SetKeepAliveConfig(keepAliveConfig)
	}
	return c, nil
}

func DialUDP(network string, laddr, raddr *net.UDPAddr) (*UDPConn, error) {
	ctx := context.Background()
	return DefaultDialer.DialUDP(ctx, network, laddr, raddr)
}

func (d *Dialer) DialUDP(ctx context.Context, network string, laddr, raddr *net.UDPAddr) (*UDPConn, error) {
	// network
	switch network {
	case "udp", "udp4", "udp6":
	default:
		return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: net.UnknownNetworkError(network)}
	}
	// vortex
	vortex, vortexErr := aio.Acquire()
	if vortexErr != nil {
		return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: vortexErr}
	}
	// fd
	now := time.Now()
	deadline := d.deadline(ctx, time.Now())
	if deadline.Before(now) {
		_ = aio.Release(vortex)
		return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: aio.Timeout}
	}

	var control ctrlCtxFn = d.ControlContext
	if control == nil && d.Control != nil {
		control = func(ctx context.Context, network string, address string, raw syscall.RawConn) error {
			return d.Control(network, address, raw)
		}
	}
	fd, fdErr := newDialerFd(ctx, network, laddr, raddr, syscall.SOCK_DGRAM, 0, false, control)
	if fdErr != nil {
		_ = aio.Release(vortex)
		return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: fdErr}
	}

	if raddr != nil { // connect
		sa, saErr := sys.AddrToSockaddr(raddr)
		if saErr != nil {
			_ = fd.Close()
			_ = aio.Release(vortex)
			return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: saErr}
		}
		rsa, rsaLen, rsaErr := sys.SockaddrToRawSockaddrAny(sa)
		if rsaErr != nil {
			_ = fd.Close()
			_ = aio.Release(vortex)
			return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: rsaErr}
		}
		future := vortex.PrepareConnect(fd.Socket(), rsa, int(rsaLen), deadline)
		_, err := future.Await(ctx)
		if err != nil {
			_ = fd.Close()
			_ = aio.Release(vortex)
			return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: err}
		}
	}

	// local addr
	if laddr != nil {
		fd.SetLocalAddr(laddr)
	} else {
		if laddrErr := fd.LoadLocalAddr(); laddrErr != nil {
			_ = fd.Close()
			_ = aio.Release(vortex)
			return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: laddrErr}
		}
	}
	// remote addr
	if raddrErr := fd.LoadRemoteAddr(); raddrErr != nil {
		fd.SetRemoteAddr(raddr)
	}

	// conn
	useSendZC := d.UseSendZC
	useSendMsgZC := false
	if useSendZC {
		useSendZC = aio.CheckSendZCEnable()
		useSendMsgZC = aio.CheckSendMsdZCEnable()
	}

	c := &UDPConn{
		conn{
			ctx:           ctx,
			fd:            fd,
			vortex:        vortex,
			readDeadline:  time.Time{},
			writeDeadline: time.Time{},
			useZC:         useSendZC,
			pinned:        true,
		},
		useSendMsgZC,
	}
	return c, nil
}

func DialUnix(network string, laddr, raddr *net.UnixAddr) (*UnixConn, error) {
	ctx := context.Background()
	return DefaultDialer.DialUnix(ctx, network, laddr, raddr)
}

func (d *Dialer) DialUnix(ctx context.Context, network string, laddr, raddr *net.UnixAddr) (*UnixConn, error) {
	// network
	sotype := 0
	switch network {
	case "unix", "unixpacket":
		sotype = syscall.SOCK_STREAM
		break
	case "unixgram":
		sotype = syscall.SOCK_DGRAM
		break
	default:
		return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: net.UnknownNetworkError(network)}
	}
	if laddr != nil && sys.IsWildcard(laddr) {
		laddr = nil
	}
	if raddr != nil && sys.IsWildcard(raddr) {
		raddr = nil
	}
	if raddr == nil && (sotype != syscall.SOCK_DGRAM || laddr == nil) {
		return nil, errors.New("missing address")
	}
	// vortex
	vortex, vortexErr := aio.Acquire()
	if vortexErr != nil {
		return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: vortexErr}
	}
	// fd
	now := time.Now()
	deadline := d.deadline(ctx, time.Now())
	if deadline.Before(now) {
		_ = aio.Release(vortex)
		return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: aio.Timeout}
	}

	var control ctrlCtxFn = d.ControlContext
	if control == nil && d.Control != nil {
		control = func(ctx context.Context, network string, address string, raw syscall.RawConn) error {
			return d.Control(network, address, raw)
		}
	}
	fd, fdErr := newDialerFd(ctx, network, laddr, raddr, sotype, 0, false, control)
	if fdErr != nil {
		_ = aio.Release(vortex)
		return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: fdErr}
	}

	if raddr != nil { // connect
		sa, saErr := sys.AddrToSockaddr(raddr)
		if saErr != nil {
			_ = fd.Close()
			_ = aio.Release(vortex)
			return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: saErr}
		}
		rsa, rsaLen, rsaErr := sys.SockaddrToRawSockaddrAny(sa)
		if rsaErr != nil {
			_ = fd.Close()
			_ = aio.Release(vortex)
			return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: rsaErr}
		}
		future := vortex.PrepareConnect(fd.Socket(), rsa, int(rsaLen), deadline)
		_, err := future.Await(ctx)
		if err != nil {
			_ = fd.Close()
			_ = aio.Release(vortex)
			return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: err}
		}
	}

	// local addr
	if laddr != nil {
		fd.SetLocalAddr(laddr)
	} else {
		if laddrErr := fd.LoadLocalAddr(); laddrErr != nil {
			_ = fd.Close()
			_ = aio.Release(vortex)
			return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: laddrErr}
		}
	}
	// remote addr
	if raddrErr := fd.LoadRemoteAddr(); raddrErr != nil {
		fd.SetRemoteAddr(raddr)
	}

	// conn
	useSendZC := d.UseSendZC
	useSendMsgZC := false
	if useSendZC {
		useSendZC = aio.CheckSendZCEnable()
		useSendMsgZC = aio.CheckSendMsdZCEnable()
	}

	c := &UnixConn{
		conn{
			ctx:           ctx,
			fd:            fd,
			vortex:        vortex,
			readDeadline:  time.Time{},
			writeDeadline: time.Time{},
			useZC:         useSendZC,
			pinned:        true,
		},
		useSendMsgZC,
	}

	return c, nil
}

func DialIP(network string, laddr, raddr *net.IPAddr) (*IPConn, error) {
	ctx := context.Background()
	return DefaultDialer.DialIP(ctx, network, laddr, raddr)
}

func (d *Dialer) DialIP(_ context.Context, network string, laddr, raddr *net.IPAddr) (*IPConn, error) {
	c, err := net.DialIP(network, laddr, raddr)
	if err != nil {
		return nil, err
	}
	return &IPConn{c}, nil
}

func newDialerFd(ctx context.Context, network string, laddr net.Addr, raddr net.Addr, sotype int, proto int, fastOpen bool, control ctrlCtxFn) (fd *sys.Fd, err error) {
	if reflect.ValueOf(laddr).IsNil() && reflect.ValueOf(raddr).IsNil() {
		err = errors.New("missing address")
		return
	}
	addr := raddr
	if reflect.ValueOf(raddr).IsNil() {
		addr = laddr
	}
	_, family, ipv6only, addrErr := sys.ResolveAddr(network, addr.String())
	if addrErr != nil {
		err = addrErr
		return
	}
	// fd
	sock, sockErr := sys.NewSocket(family, sotype, proto)
	if sockErr != nil {
		err = sockErr
		return
	}
	fd = sys.NewFd(network, sock, family, sotype)
	// ipv6
	if ipv6only {
		if err = fd.SetIpv6only(true); err != nil {
			_ = fd.Close()
			return
		}
	}
	// reuse addr
	if err = fd.AllowReuseAddr(); err != nil {
		_ = fd.Close()
		return
	}
	// broadcast
	if err = fd.AllowBroadcast(); err != nil {
		_ = fd.Close()
		return
	}
	// fast open
	if fastOpen {
		if err = fd.AllowFastOpen(fastOpen); err != nil {
			_ = fd.Close()
			return
		}
	}
	// control
	if control != nil {
		raw := newRawConn(fd)
		address := ""
		if !reflect.ValueOf(raddr).IsNil() {
			address = raddr.String()
		}
		if err = control(ctx, network, address, raw); err != nil {
			_ = fd.Close()
			return
		}
	}
	// bind
	if !reflect.ValueOf(laddr).IsNil() {
		if err = fd.Bind(laddr); err != nil {
			_ = fd.Close()
			return
		}
	}
	return
}
