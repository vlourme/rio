//go:build linux

package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"github.com/brickingsoft/rio/pkg/sys"
	"net"
	"net/netip"
	"os"
	"reflect"
	"sync/atomic"
	"syscall"
	"time"
)

// ListenUDP acts like [ListenPacket] for UDP networks.
//
// The network must be a UDP network name; see func [Dial] for details.
//
// If the IP field of laddr is nil or an unspecified IP address,
// ListenUDP listens on all available IP addresses of the local system
// except multicast IP addresses.
// If the Port field of laddr is 0, a port number is automatically
// chosen.
func ListenUDP(network string, addr *net.UDPAddr) (*UDPConn, error) {
	config := ListenConfig{}
	ctx := context.Background()
	return config.ListenUDP(ctx, network, addr)
}

// ListenUDP acts like [ListenPacket] for UDP networks.
//
// The network must be a UDP network name; see func [Dial] for details.
//
// If the IP field of laddr is nil or an unspecified IP address,
// ListenUDP listens on all available IP addresses of the local system
// except multicast IP addresses.
// If the Port field of laddr is 0, a port number is automatically
// chosen.
func (lc *ListenConfig) ListenUDP(ctx context.Context, network string, addr *net.UDPAddr) (*UDPConn, error) {
	return lc.listenUDP(ctx, network, nil, addr)
}

// ListenMulticastUDP acts like [ListenPacket] for UDP networks but
// takes a group address on a specific network interface.
//
// The network must be a UDP network name; see func [Dial] for details.
//
// ListenMulticastUDP listens on all available IP addresses of the
// local system including the group, multicast IP address.
// If ifi is nil, ListenMulticastUDP uses the system-assigned
// multicast interface, although this is not recommended because the
// assignment depends on platforms and sometimes it might require
// routing configuration.
// If the Port field of gaddr is 0, a port number is automatically
// chosen.
//
// ListenMulticastUDP is just for convenience of simple, small
// applications. There are [golang.org/x/net/ipv4] and
// [golang.org/x/net/ipv6] packages for general purpose uses.
//
// Note that ListenMulticastUDP will set the IP_MULTICAST_LOOP socket option
// to 0 under IPPROTO_IP, to disable loopback of multicast packets.
func ListenMulticastUDP(network string, ifi *net.Interface, addr *net.UDPAddr) (*UDPConn, error) {
	config := ListenConfig{}
	ctx := context.Background()
	return config.ListenMulticastUDP(ctx, network, ifi, addr)
}

// ListenMulticastUDP acts like [ListenPacket] for UDP networks but
// takes a group address on a specific network interface.
//
// The network must be a UDP network name; see func [Dial] for details.
//
// ListenMulticastUDP listens on all available IP addresses of the
// local system including the group, multicast IP address.
// If ifi is nil, ListenMulticastUDP uses the system-assigned
// multicast interface, although this is not recommended because the
// assignment depends on platforms and sometimes it might require
// routing configuration.
// If the Port field of gaddr is 0, a port number is automatically
// chosen.
//
// ListenMulticastUDP is just for convenience of simple, small
// applications. There are [golang.org/x/net/ipv4] and
// [golang.org/x/net/ipv6] packages for general purpose uses.
//
// Note that ListenMulticastUDP will set the IP_MULTICAST_LOOP socket option
// to 0 under IPPROTO_IP, to disable loopback of multicast packets.
func (lc *ListenConfig) ListenMulticastUDP(ctx context.Context, network string, ifi *net.Interface, addr *net.UDPAddr) (*UDPConn, error) {
	return lc.listenUDP(ctx, network, ifi, addr)
}

func (lc *ListenConfig) listenUDP(ctx context.Context, network string, ifi *net.Interface, addr *net.UDPAddr) (*UDPConn, error) {
	// network
	switch network {
	case "udp", "udp4", "udp6":
	default:
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: net.UnknownNetworkError(network)}
	}
	if addr == nil {
		addr = &net.UDPAddr{}
	}
	// vortex
	vortex := lc.Vortex
	if vortex == nil {
		var vortexErr error
		vortex, vortexErr = getVortex()
		if vortexErr != nil {
			return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: vortexErr}
		}
	}
	// fd
	fd, fdErr := newUDPListenerFd(network, ifi, addr)
	if fdErr != nil {
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: fdErr}
	}
	// install fixed fd
	fileIndex := -1
	sqeFlags := uint8(0)
	if vortex.RegisterFixedFdEnabled() {
		sock := fd.Socket()
		file, regErr := vortex.RegisterFixedFd(ctx, sock)
		if regErr == nil {
			fileIndex = file
			sqeFlags = iouring.SQEFixedFile
		} else {
			if !errors.Is(regErr, aio.ErrFixedFileUnavailable) {
				_ = fd.Close()
				return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: regErr}
			}
		}
	}
	// send zc
	useSendZC := false
	useSendMSGZC := false
	if lc.SendZC {
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
			useSendZC:     useSendZC,
		},
		useSendMSGZC,
	}
	return c, nil
}

func newUDPListenerFd(network string, ifi *net.Interface, addr *net.UDPAddr) (fd *sys.Fd, err error) {
	resolveAddr, family, ipv6only, addrErr := sys.ResolveAddr(network, addr.String())
	if addrErr != nil {
		err = addrErr
		return
	}
	// fd
	sock, sockErr := sys.NewSocket(family, syscall.SOCK_DGRAM, 0)
	if sockErr != nil {
		err = sockErr
		return
	}
	fd = sys.NewFd(network, sock, family, syscall.SOCK_DGRAM)
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
	// multicast
	isListenMulticastUDP := false
	var gaddr *net.UDPAddr
	if addr.IP != nil && addr.IP.IsMulticast() {
		if err = fd.AllowReuseAddr(); err != nil {
			_ = fd.Close()
			return
		}
		isListenMulticastUDP = true
		gaddr = addr
		localUdpAddr := *addr
		switch family {
		case syscall.AF_INET:
			localUdpAddr.IP = net.IPv4zero.To4()
		case syscall.AF_INET6:
			localUdpAddr.IP = net.IPv6zero
		}
		addr = &localUdpAddr
	}
	if isListenMulticastUDP {
		if ip4 := gaddr.IP.To4(); ip4 != nil {
			if ifi != nil {
				if err = fd.SetIPv4MulticastInterface(ifi); err != nil {
					_ = fd.Close()
					return
				}
			}
			if err = fd.SetIPv4MulticastLoopback(false); err != nil {
				_ = fd.Close()
				return
			}
			if err = fd.JoinIPv4Group(ifi, ip4); err != nil {
				_ = fd.Close()
				return
			}
		} else {
			if ifi != nil {
				if err = fd.SetIPv6MulticastInterface(ifi); err != nil {
					_ = fd.Close()
					return
				}
			}
			if err = fd.SetIPv6MulticastLoopback(false); err != nil {
				_ = fd.Close()
				return
			}
			if err = fd.JoinIPv6Group(ifi, gaddr.IP); err != nil {
				_ = fd.Close()
				return
			}
		}
	}
	// bind
	sa, saErr := sys.AddrToSockaddr(addr)
	if saErr != nil {
		_ = fd.Close()
		err = saErr
		return
	}
	bindErr := syscall.Bind(sock, sa)
	if bindErr != nil {
		_ = fd.Close()
		err = os.NewSyscallError("bind", bindErr)
		return
	}

	// set socket addr
	if sn, getSockNameErr := syscall.Getsockname(sock); getSockNameErr == nil {
		if sockname := sys.SockaddrToAddr(network, sn); sockname != nil {
			fd.SetLocalAddr(sockname)
		} else {
			fd.SetLocalAddr(resolveAddr)
		}
	} else {
		fd.SetLocalAddr(resolveAddr)
	}
	return
}

// UDPConn is the implementation of the [net.Conn] and [net.PacketConn] interfaces
// for UDP network connections.
type UDPConn struct {
	conn
	useSendMSGZC bool
}

// UseSendMSGZC try to enable sendmsg_zc.
func (c *UDPConn) UseSendMSGZC(use bool) bool {
	if !c.ok() {
		return false
	}
	if use {
		use = aio.CheckSendMsdZCEnable()
	}
	c.useSendMSGZC = use
	return use
}

// ReadFromUDP acts like [UDPConn.ReadFrom] but returns a net.UDPAddr.
func (c *UDPConn) ReadFromUDP(b []byte) (n int, addr *net.UDPAddr, err error) {
	if !c.ok() {
		return 0, nil, syscall.EINVAL
	}
	if len(b) == 0 {
		return 0, nil, &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}

	ctx := c.ctx
	fd := 0
	if c.fdFixed {
		fd = c.fileIndex
	} else {
		fd = c.fd.Socket()
	}
	vortex := c.vortex

	rsa := &syscall.RawSockaddrAny{}
	rsaLen := syscall.SizeofSockaddrAny

	deadline := c.deadline(ctx, c.readDeadline)

	n, err = vortex.ReceiveFrom(ctx, fd, b, rsa, rsaLen, deadline, c.sqeFlags)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			err = net.ErrClosed
		}
		err = &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
		return
	}

	sa, saErr := sys.RawSockaddrAnyToSockaddr(rsa)
	if saErr != nil {
		err = &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: saErr}
		return
	}
	a := sys.SockaddrToAddr(c.fd.Net(), sa)
	ok := false
	addr, ok = a.(*net.UDPAddr)
	if !ok {
		err = &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: errors.New("wrong address type")}
		return
	}
	return
}

// ReadFrom implements the [net.PacketConn] ReadFrom method.
func (c *UDPConn) ReadFrom(b []byte) (n int, addr net.Addr, err error) {
	return c.ReadFromUDP(b)
}

// ReadFromUDPAddrPort acts like ReadFrom but returns a [netip.AddrPort].
//
// If c is bound to an unspecified address, the returned
// netip.AddrPort's address might be an IPv4-mapped IPv6 address.
// Use [netip.Addr.Unmap] to get the address without the IPv6 prefix.
func (c *UDPConn) ReadFromUDPAddrPort(b []byte) (n int, addr netip.AddrPort, err error) {
	var udpAddr *net.UDPAddr
	n, udpAddr, err = c.ReadFromUDP(b)
	if err != nil {
		return
	}
	addr = udpAddr.AddrPort()
	return
}

// ReadMsgUDP reads a message from c, copying the payload into b and
// the associated out-of-band data into oob. It returns the number of
// bytes copied into b, the number of bytes copied into oob, the flags
// that were set on the message and the source address of the message.
//
// The packages [golang.org/x/net/ipv4] and [golang.org/x/net/ipv6] can be
// used to manipulate IP-level socket options in oob.
func (c *UDPConn) ReadMsgUDP(b, oob []byte) (n, oobn, flags int, addr *net.UDPAddr, err error) {
	if !c.ok() {
		return 0, 0, 0, nil, syscall.EINVAL
	}
	bLen := len(b)
	oobLen := len(oob)
	if bLen == 0 && oobLen == 0 {
		return 0, 0, 0, nil, &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}

	ctx := c.ctx
	fd := 0
	if c.fdFixed {
		fd = c.fileIndex
	} else {
		fd = c.fd.Socket()
	}
	vortex := c.vortex

	rsa := &syscall.RawSockaddrAny{}
	rsaLen := syscall.SizeofSockaddrAny

	deadline := c.deadline(ctx, c.readDeadline)

	n, oobn, flags, err = vortex.ReceiveMsg(ctx, fd, b, oob, rsa, rsaLen, 0, deadline, c.sqeFlags)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			err = net.ErrClosed
		}
		err = &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
		return
	}

	sa, saErr := sys.RawSockaddrAnyToSockaddr(rsa)
	if saErr != nil {
		err = &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: saErr}
		return
	}
	a := sys.SockaddrToAddr(c.fd.Net(), sa)
	ok := false
	addr, ok = a.(*net.UDPAddr)
	if !ok {
		err = &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: net.InvalidAddrError("wrong address type")}
		return
	}
	return
}

// ReadMsgUDPAddrPort is like [UDPConn.ReadMsgUDP] but returns an [netip.AddrPort] instead of a [net.UDPAddr].
func (c *UDPConn) ReadMsgUDPAddrPort(b, oob []byte) (n, oobn, flags int, addr netip.AddrPort, err error) {
	var udpAddr *net.UDPAddr
	n, oobn, flags, udpAddr, err = c.ReadMsgUDP(b, oob)
	if err != nil {
		return
	}
	addr = udpAddr.AddrPort()
	return
}

// WriteToUDP acts like [UDPConn.WriteTo] but takes a [UDPAddr].
func (c *UDPConn) WriteToUDP(b []byte, addr *net.UDPAddr) (n int, err error) {
	n, err = c.WriteTo(b, addr)
	return
}

// WriteToUDPAddrPort acts like [UDPConn.WriteTo] but takes a [netip.AddrPort].
func (c *UDPConn) WriteToUDPAddrPort(b []byte, addr netip.AddrPort) (n int, err error) {
	if !c.ok() {
		return 0, syscall.EINVAL
	}
	if len(b) == 0 || !addr.IsValid() {
		return 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}
	sa, saErr := sys.AddrPortToSockaddr(addr)
	if saErr != nil {
		return 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: saErr}
	}
	n, err = c.writeTo(b, sa)
	return
}

// WriteTo implements the [net.PacketConn] WriteTo method.
func (c *UDPConn) WriteTo(b []byte, addr net.Addr) (n int, err error) {
	if !c.ok() {
		return 0, syscall.EINVAL
	}
	if len(b) == 0 || reflect.ValueOf(addr).IsNil() {
		return 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}
	uAddr, addrOk := addr.(*net.UDPAddr)
	if !addrOk {
		return 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}
	sa, saErr := sys.AddrToSockaddr(uAddr)
	if saErr != nil {
		return 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: saErr}
	}
	n, err = c.writeTo(b, sa)
	return
}

func (c *UDPConn) writeTo(b []byte, addr syscall.Sockaddr) (n int, err error) {
	rsa, rsaLen, rsaErr := sys.SockaddrToRawSockaddrAny(addr)
	if rsaErr != nil {
		return 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: rsaErr}
	}

	ctx := c.ctx
	fd := 0
	if c.fdFixed {
		fd = c.fileIndex
	} else {
		fd = c.fd.Socket()
	}
	vortex := c.vortex

	deadline := c.deadline(ctx, c.writeDeadline)

	if c.useSendMSGZC {
		n, err = vortex.SendToZC(ctx, fd, b, rsa, int(rsaLen), deadline, c.sqeFlags)
	} else {
		n, err = vortex.SendTo(ctx, fd, b, rsa, int(rsaLen), deadline, c.sqeFlags)
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			err = net.ErrClosed
		}
		err = &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
		return
	}
	return
}

// WriteMsgUDP writes a message to addr via c if c isn't connected, or
// to c's remote address if c is connected (in which case addr must be
// nil). The payload is copied from b and the associated out-of-band
// data is copied from oob. It returns the number of payload and
// out-of-band bytes written.
//
// The packages [golang.org/x/net/ipv4] and [golang.org/x/net/ipv6] can be
// used to manipulate IP-level socket options in oob.
func (c *UDPConn) WriteMsgUDP(b, oob []byte, addr *net.UDPAddr) (n, oobn int, err error) {
	if !c.ok() {
		return 0, 0, syscall.EINVAL
	}
	if len(b) == 0 && len(oob) == 0 {
		return 0, 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}
	if addr == nil {
		return 0, 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}
	sa, saErr := sys.AddrToSockaddr(addr)
	if saErr != nil {
		return 0, 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: saErr}
	}
	n, oobn, err = c.writeMsg(b, oob, sa)
	return
}

// WriteMsgUDPAddrPort is like [UDPConn.WriteMsgUDP] but takes a [netip.AddrPort] instead of a [net.UDPAddr].
func (c *UDPConn) WriteMsgUDPAddrPort(b, oob []byte, addr netip.AddrPort) (n, oobn int, err error) {
	if !c.ok() {
		return 0, 0, syscall.EINVAL
	}
	if len(b) == 0 && len(oob) == 0 {
		return 0, 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}
	if !addr.IsValid() {
		return 0, 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}
	sa, saErr := sys.AddrPortToSockaddr(addr)
	if saErr != nil {
		return 0, 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: saErr}
	}
	n, oobn, err = c.writeMsg(b, oob, sa)
	return
}

func (c *UDPConn) writeMsg(b, oob []byte, addr syscall.Sockaddr) (n, oobn int, err error) {
	rsa, rsaLen, rsaErr := sys.SockaddrToRawSockaddrAny(addr)
	if rsaErr != nil {
		err = &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: rsaErr}
		return
	}

	if len(b) == 0 && c.fd.SocketType() != syscall.SOCK_DGRAM {
		b = []byte{0}
	}

	ctx := c.ctx
	fd := 0
	if c.fdFixed {
		fd = c.fileIndex
	} else {
		fd = c.fd.Socket()
	}
	vortex := c.vortex

	deadline := c.deadline(ctx, c.writeDeadline)
	if c.useSendMSGZC {
		n, oobn, err = vortex.SendMsgZC(ctx, fd, b, oob, rsa, int(rsaLen), deadline, c.sqeFlags)
	} else {
		n, oobn, err = vortex.SendMsg(ctx, fd, b, oob, rsa, int(rsaLen), deadline, c.sqeFlags)
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			err = net.ErrClosed
		}
		err = &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
		return
	}
	return
}
