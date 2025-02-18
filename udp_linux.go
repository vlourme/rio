//go:build linux

package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"github.com/brickingsoft/rio/pkg/sys"
	"net"
	"net/netip"
	"os"
	"sync/atomic"
	"syscall"
)

func ListenUDP(network string, addr *net.UDPAddr) (*UDPConn, error) {
	config := ListenConfig{
		UseSendZC: defaultUseSendZC.Load(),
	}
	ctx := context.Background()
	return config.ListenUDP(ctx, network, addr)
}

func (lc *ListenConfig) ListenUDP(ctx context.Context, network string, addr *net.UDPAddr) (*UDPConn, error) {
	return lc.listenUDP(ctx, network, nil, addr)
}

func ListenMulticastUDP(network string, ifi *net.Interface, addr *net.UDPAddr) (*UDPConn, error) {
	config := ListenConfig{}
	ctx := context.Background()
	return config.ListenMulticastUDP(ctx, network, ifi, addr)
}

func (lc *ListenConfig) ListenMulticastUDP(ctx context.Context, network string, ifi *net.Interface, addr *net.UDPAddr) (*UDPConn, error) {
	return lc.listenUDP(ctx, network, ifi, addr)
}

func (lc *ListenConfig) listenUDP(ctx context.Context, network string, ifi *net.Interface, addr *net.UDPAddr) (*UDPConn, error) {
	// vortex
	vortex, vortexErr := getCenterVortex()
	if vortexErr != nil {
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: vortexErr}
	}
	// fd
	switch network {
	case "udp", "udp4", "udp6":
	default:
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: net.UnknownNetworkError(network)}
	}
	if addr == nil {
		addr = &net.UDPAddr{}
	}
	fd, fdErr := newUDPListenerFd(network, ifi, addr)
	if fdErr != nil {
		return nil, &net.OpError{Op: "listen", Net: network, Source: nil, Addr: addr, Err: fdErr}
	}
	// ctx
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	// vortex start
	vortex.Start(ctx)
	// sendzc
	useSendZC := lc.UseSendZC
	if useSendZC {
		useSendZC = aio.CheckSendMsdZCEnable()
	}
	// conn
	conn := &UDPConn{
		connection{
			ctx:          ctx,
			cancel:       cancel,
			fd:           fd,
			vortex:       vortex,
			readTimeout:  atomic.Int64{},
			writeTimeout: atomic.Int64{},
			useZC:        useSendZC,
		},
	}
	return conn, nil
}

func newUDPListenerFd(network string, ifi *net.Interface, addr *net.UDPAddr) (fd *sys.Fd, err error) {
	_, family, ipv6only, addrErr := sys.ResolveAddr(network, addr.String())
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
	return
}

type UDPConn struct {
	connection
}

func (c *UDPConn) SyscallConn() (syscall.RawConn, error) {
	return newRawConnection(c.fd), nil
}

func (c *UDPConn) ReadFromUDP(b []byte) (n int, addr *net.UDPAddr, err error) {

	return
}

func (c *UDPConn) ReadFrom(b []byte) (n int, addr net.Addr, err error) {

	return
}

func (c *UDPConn) ReadFromUDPAddrPort(b []byte) (n int, addr netip.AddrPort, err error) {

	return
}

func (c *UDPConn) ReadMsgUDP(b, oob []byte) (n, oobn, flags int, addr *net.UDPAddr, err error) {

	return
}

func (c *UDPConn) ReadMsgUDPAddrPort(b, oob []byte) (n, oobn, flags int, addr netip.AddrPort, err error) {

	return
}

func (c *UDPConn) WriteToUDP(b []byte, addr *net.UDPAddr) (n int, err error) {

	return
}

func (c *UDPConn) WriteToUDPAddrPort(b []byte, addr netip.AddrPort) (n int, err error) {

	return
}

func (c *UDPConn) WriteTo(b []byte, addr net.Addr) (n int, err error) {

	return
}

func (c *UDPConn) WriteMsgUDP(b, oob []byte, addr *net.UDPAddr) (n, oobn int, err error) {

	return
}

func (c *UDPConn) WriteMsgUDPAddrPort(b, oob []byte, addr netip.AddrPort) (n, oobn int, err error) {

	return
}
