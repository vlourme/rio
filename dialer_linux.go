//go:build linux

package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"github.com/brickingsoft/rio/pkg/sys"
	"net"
	"reflect"
	"sync/atomic"
	"syscall"
	"time"
)

// DialContext connects to the address on the named network using
// the provided context.
//
// The provided Context must be non-nil. If the context expires before
// the connection is complete, an error is returned. Once successfully
// connected, any expiration of the context will not affect the
// connection.
//
// When using TCP, and the host in the address parameter resolves to multiple
// network addresses, any dial timeout (from d.Timeout or ctx) is spread
// over each consecutive dial, such that each is given an appropriate
// fraction of the time to connect.
// For example, if a host has 4 IP addresses and the timeout is 1 minute,
// the connect to each single address will be given 15 seconds to complete
// before trying the next one.
//
// See func [Dial] for a description of the network and address
// parameters.
func (d *Dialer) DialContext(ctx context.Context, network, address string) (c net.Conn, err error) {
	addr, _, _, addrErr := sys.ResolveAddr(network, address)
	if addrErr != nil {
		err = &net.OpError{Op: "dial", Net: network, Source: nil, Addr: nil, Err: addrErr}
		return
	}
	la := d.LocalAddr
	switch a := addr.(type) {
	case *net.TCPAddr:
		d.SetFastOpen(true)
		d.SetQuickAck(true)
		la, _ := la.(*net.TCPAddr)
		c, err = d.DialTCP(ctx, network, la, a)
		break
	case *net.UDPAddr:
		la, _ := la.(*net.UDPAddr)
		c, err = d.DialUDP(ctx, network, la, a)
		break
	case *net.UnixAddr:
		la, _ := la.(*net.UnixAddr)
		c, err = d.DialUnix(ctx, network, la, a)
		break
	case *net.IPAddr:
		la, _ := la.(*net.IPAddr)
		c, err = d.DialIP(ctx, network, la, a)
		break
	default:
		err = &net.OpError{Op: "dial", Net: network, Source: nil, Addr: addr, Err: &net.AddrError{Err: "unexpected address type", Addr: address}}
		break
	}
	return
}

// Dial connects to the address on the named network.
//
// See func Dial for a description of the network and address
// parameters.
//
// Dial uses [context.Background] internally; to specify the context, use
// [Dialer.DialContext].
func (d *Dialer) Dial(network string, address string) (c net.Conn, err error) {
	ctx := context.Background()
	return d.DialContext(ctx, network, address)
}

// DialTCP acts like [Dial] for TCP networks.
//
// The network must be a TCP network name; see func Dial for details.
//
// If laddr is nil, a local address is automatically chosen.
// If the IP field of raddr is nil or an unspecified IP address, the
// local system is assumed.
func DialTCP(network string, laddr, raddr *net.TCPAddr) (*TCPConn, error) {
	ctx := context.Background()
	dialer := DefaultDialer
	return dialer.DialTCP(ctx, network, laddr, raddr)
}

// DialTCP acts like [Dial] for TCP networks.
//
// The network must be a TCP network name; see func Dial for details.
//
// If laddr is nil, a local address is automatically chosen.
// If the IP field of raddr is nil or an unspecified IP address, the
// local system is assumed.
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
		return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: aio.ErrTimeout}
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
	_, err := vortex.Connect(ctx, fd.Socket(), rsa, int(rsaLen), deadline, 0)
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

	// no delay
	_ = fd.SetNoDelay(true)
	// keepalive
	keepAliveConfig := d.KeepAliveConfig
	if !keepAliveConfig.Enable && d.KeepAlive >= 0 {
		keepAliveConfig = net.KeepAliveConfig{
			Enable: true,
			Idle:   d.KeepAlive,
		}
	}
	if keepAliveConfig.Enable {
		_ = fd.SetKeepAliveConfig(keepAliveConfig)
	}

	// install fixed fd
	fileIndex := -1
	sqeFlags := uint8(0)
	if d.AutoFixedFdInstall && vortex.RegisterFixedFdEnabled() {
		sock := fd.Socket()
		file, regErr := vortex.RegisterFixedFd(ctx, sock)
		if regErr == nil {
			fileIndex = file
			sqeFlags = iouring.SQEFixedFile
		} else {
			if !errors.Is(regErr, aio.ErrFixedFileUnavailable) {
				_ = fd.Close()
				_ = aio.Release(vortex)
				return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: regErr}
			}
		}
	}

	// send zc
	useSendZC := false
	if d.SendZC {
		useSendZC = aio.CheckSendZCEnable()
	}
	// conn
	cc, cancel := context.WithCancel(ctx)
	c := &TCPConn{
		conn{
			ctx:           cc,
			cancel:        cancel,
			fd:            fd,
			fdFixed:       fileIndex != -1,
			fileIndex:     fileIndex,
			sqeFlags:      sqeFlags,
			vortex:        vortex,
			readDeadline:  time.Time{},
			writeDeadline: time.Time{},
			readBuffer:    atomic.Int64{},
			writeBuffer:   atomic.Int64{},
			pinned:        true,
			useSendZC:     useSendZC,
		},
		0,
	}

	return c, nil
}

// DialUDP acts like [Dial] for UDP networks.
//
// The network must be a UDP network name; see func [Dial] for details.
//
// If laddr is nil, a local address is automatically chosen.
// If the IP field of raddr is nil or an unspecified IP address, the
// local system is assumed.
func DialUDP(network string, laddr, raddr *net.UDPAddr) (*UDPConn, error) {
	ctx := context.Background()
	return DefaultDialer.DialUDP(ctx, network, laddr, raddr)
}

// DialUDP acts like [Dial] for UDP networks.
//
// The network must be a UDP network name; see func [Dial] for details.
//
// If laddr is nil, a local address is automatically chosen.
// If the IP field of raddr is nil or an unspecified IP address, the
// local system is assumed.
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
		return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: aio.ErrTimeout}
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
		_, err := vortex.Connect(ctx, fd.Socket(), rsa, int(rsaLen), deadline, 0)
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

	// install fixed fd
	fileIndex := -1
	sqeFlags := uint8(0)
	if d.AutoFixedFdInstall && vortex.RegisterFixedFdEnabled() {
		sock := fd.Socket()
		file, regErr := vortex.RegisterFixedFd(ctx, sock)
		if regErr == nil {
			fileIndex = file
			sqeFlags = iouring.SQEFixedFile
		} else {
			if !errors.Is(regErr, aio.ErrFixedFileUnavailable) {
				_ = fd.Close()
				_ = aio.Release(vortex)
				return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: regErr}
			}
		}
	}

	// send zc
	useSendZC := false
	useSendMSGZC := false
	if d.SendZC {
		useSendZC = aio.CheckSendZCEnable()
		useSendMSGZC = aio.CheckSendMsdZCEnable()
	}
	// conn
	cc, cancel := context.WithCancel(ctx)
	c := &UDPConn{
		conn{
			ctx:           cc,
			cancel:        cancel,
			fd:            fd,
			fdFixed:       fileIndex != -1,
			fileIndex:     fileIndex,
			sqeFlags:      sqeFlags,
			vortex:        vortex,
			readDeadline:  time.Time{},
			writeDeadline: time.Time{},
			readBuffer:    atomic.Int64{},
			writeBuffer:   atomic.Int64{},
			pinned:        true,
			useSendZC:     useSendZC,
		},
		useSendMSGZC,
	}
	return c, nil
}

// DialUnix acts like [Dial] for Unix networks.
//
// The network must be a Unix network name; see func Dial for details.
//
// If laddr is non-nil, it is used as the local address for the
// connection.
func DialUnix(network string, laddr, raddr *net.UnixAddr) (*UnixConn, error) {
	ctx := context.Background()
	return DefaultDialer.DialUnix(ctx, network, laddr, raddr)
}

// DialUnix acts like [Dial] for Unix networks.
//
// The network must be a Unix network name; see func Dial for details.
//
// If laddr is non-nil, it is used as the local address for the
// connection.
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
		return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: aio.ErrTimeout}
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
		_, err := vortex.Connect(ctx, fd.Socket(), rsa, int(rsaLen), deadline, 0)
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

	// install fixed fd
	fileIndex := -1
	sqeFlags := uint8(0)
	if d.AutoFixedFdInstall && vortex.RegisterFixedFdEnabled() {
		sock := fd.Socket()
		file, regErr := vortex.RegisterFixedFd(ctx, sock)
		if regErr == nil {
			fileIndex = file
			sqeFlags = iouring.SQEFixedFile
		} else {
			if !errors.Is(regErr, aio.ErrFixedFileUnavailable) {
				_ = fd.Close()
				_ = aio.Release(vortex)
				return nil, &net.OpError{Op: "dial", Net: network, Source: laddr, Addr: raddr, Err: regErr}
			}
		}
	}

	// send zc
	useSendZC := false
	useSendMSGZC := false
	if d.SendZC {
		useSendZC = aio.CheckSendZCEnable()
		useSendMSGZC = aio.CheckSendMsdZCEnable()
	}
	// conn
	cc, cancel := context.WithCancel(ctx)
	c := &UnixConn{
		conn{
			ctx:           cc,
			cancel:        cancel,
			fd:            fd,
			fdFixed:       fileIndex != -1,
			fileIndex:     fileIndex,
			sqeFlags:      sqeFlags,
			vortex:        vortex,
			readDeadline:  time.Time{},
			writeDeadline: time.Time{},
			readBuffer:    atomic.Int64{},
			writeBuffer:   atomic.Int64{},
			pinned:        true,
			useSendZC:     useSendZC,
		},
		useSendMSGZC,
	}

	return c, nil
}

// DialIP acts like [Dial] for IP networks.
//
// The network must be an IP network name; see func Dial for details.
//
// If laddr is nil, a local address is automatically chosen.
// If the IP field of raddr is nil or an unspecified IP address, the
// local system is assumed.
func DialIP(network string, laddr, raddr *net.IPAddr) (*IPConn, error) {
	ctx := context.Background()
	return DefaultDialer.DialIP(ctx, network, laddr, raddr)
}

// DialIP acts like [Dial] for IP networks.
//
// The network must be an IP network name; see func Dial for details.
//
// If laddr is nil, a local address is automatically chosen.
// If the IP field of raddr is nil or an unspecified IP address, the
// local system is assumed.
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
