//go:build linux

package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"github.com/brickingsoft/rio/pkg/sys"
	"net"
	"net/netip"
	"os"
	"sync/atomic"
	"syscall"
	"time"
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
	c := &UDPConn{
		conn{
			ctx:          ctx,
			cancel:       cancel,
			fd:           fd,
			vortex:       vortex,
			readTimeout:  atomic.Int64{},
			writeTimeout: atomic.Int64{},
			useZC:        useSendZC,
		},
	}
	return c, nil
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
	conn
}

func (c *UDPConn) SyscallConn() (syscall.RawConn, error) {
	if !c.ok() {
		return nil, syscall.EINVAL
	}
	return newRawConn(c.fd), nil
}

func (c *UDPConn) ReadFromUDP(b []byte) (n int, addr *net.UDPAddr, err error) {
	if !c.ok() {
		return 0, nil, syscall.EINVAL
	}
	if len(b) == 0 {
		return 0, nil, syscall.EFAULT
	}

	ctx := c.ctx
	fd := c.fd.Socket()
	vortex := c.vortex

	rsa := &syscall.RawSockaddrAny{}
	rsaLen := syscall.SizeofSockaddrAny

	future := vortex.PrepareReceiveMsg(ctx, fd, b, nil, rsa, rsaLen, 0, time.Duration(c.readTimeout.Load()))
	n, err = future.Await(ctx)
	if err != nil {
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

func (c *UDPConn) ReadFrom(b []byte) (n int, addr net.Addr, err error) {
	return c.ReadFromUDP(b)
}

func (c *UDPConn) ReadFromUDPAddrPort(b []byte) (n int, addr netip.AddrPort, err error) {
	var udpAddr *net.UDPAddr
	n, udpAddr, err = c.ReadFromUDP(b)
	if err != nil {
		return
	}
	addr = udpAddr.AddrPort()
	return
}

func (c *UDPConn) ReadMsgUDP(b, oob []byte) (n, oobn, flags int, addr *net.UDPAddr, err error) {
	if !c.ok() {
		return 0, 0, 0, nil, syscall.EINVAL
	}
	bLen := len(b)
	oobLen := len(oob)
	if bLen == 0 && oobLen == 0 {
		return 0, 0, 0, nil, syscall.EFAULT
	}

	ctx := c.ctx
	fd := c.fd.Socket()
	vortex := c.vortex

	rsa := &syscall.RawSockaddrAny{}
	rsaLen := syscall.SizeofSockaddrAny

	future := vortex.PrepareReceiveMsg(ctx, fd, b, oob, rsa, rsaLen, 0, time.Duration(c.readTimeout.Load()))
	rn, msg, rErr := future.AwaitMsg(ctx)
	if rErr != nil {
		err = &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: rErr}
		return
	}

	n = rn
	oobn = int(msg.Controllen)
	flags = int(msg.Flags)

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

func (c *UDPConn) ReadMsgUDPAddrPort(b, oob []byte) (n, oobn, flags int, addr netip.AddrPort, err error) {
	var udpAddr *net.UDPAddr
	n, oobn, flags, udpAddr, err = c.ReadMsgUDP(b, oob)
	if err != nil {
		return
	}
	addr = udpAddr.AddrPort()
	return
}

func (c *UDPConn) WriteToUDP(b []byte, addr *net.UDPAddr) (n int, err error) {
	n, err = c.WriteTo(b, addr)
	return
}

func (c *UDPConn) WriteToUDPAddrPort(b []byte, addr netip.AddrPort) (n int, err error) {
	if !c.ok() {
		return 0, syscall.EINVAL
	}
	if len(b) == 0 {
		return 0, syscall.EFAULT
	}
	sa, saErr := sys.AddrPortToSockaddr(addr)
	if saErr != nil {
		return 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: saErr}
	}
	n, err = c.writeTo(b, sa)
	return
}

func (c *UDPConn) WriteTo(b []byte, addr net.Addr) (n int, err error) {
	if !c.ok() {
		return 0, syscall.EINVAL
	}
	if len(b) == 0 {
		return 0, syscall.EFAULT
	}
	sa, saErr := sys.AddrToSockaddr(addr)
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
	fd := c.fd.Socket()
	vortex := c.vortex

	if c.useZC {
		future := vortex.PrepareSendMsgZC(ctx, fd, b, nil, rsa, int(rsaLen), 0, time.Duration(c.readTimeout.Load()))
		n, err = future.Await(ctx)
	} else {
		future := vortex.PrepareSendMsg(ctx, fd, b, nil, rsa, int(rsaLen), 0, time.Duration(c.readTimeout.Load()))
		n, err = future.Await(ctx)
	}

	if err != nil {
		err = &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
		return
	}

	return
}

func (c *UDPConn) WriteMsgUDP(b, oob []byte, addr *net.UDPAddr) (n, oobn int, err error) {
	if !c.ok() {
		return 0, 0, syscall.EINVAL
	}
	if len(b) == 0 && len(oob) == 0 {
		return 0, 0, syscall.EFAULT
	}
	if addr == nil {
		return 0, 0, syscall.EINVAL
	}
	sa, saErr := sys.AddrToSockaddr(addr)
	if saErr != nil {
		return 0, 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: saErr}
	}
	n, oobn, err = c.writeMsg(b, oob, sa)
	return
}

func (c *UDPConn) WriteMsgUDPAddrPort(b, oob []byte, addr netip.AddrPort) (n, oobn int, err error) {
	if !c.ok() {
		return 0, 0, syscall.EINVAL
	}
	if len(b) == 0 && len(oob) == 0 {
		return 0, 0, syscall.EFAULT
	}
	if !addr.IsValid() {
		return 0, 0, syscall.EINVAL
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
	fd := c.fd.Socket()
	vortex := c.vortex

	if c.useZC {
		future := vortex.PrepareSendMsgZC(ctx, fd, b, oob, rsa, int(rsaLen), 0, time.Duration(c.readTimeout.Load()))
		wn, msg, wErr := future.AwaitMsg(ctx)
		if wErr == nil {
			oobn = int(msg.Controllen)
		}
		n, err = wn, wErr
	} else {
		future := vortex.PrepareSendMsg(ctx, fd, b, oob, rsa, int(rsaLen), 0, time.Duration(c.readTimeout.Load()))
		wn, msg, wErr := future.AwaitMsg(ctx)
		if wErr == nil {
			oobn = int(msg.Controllen)
		}
		n, err = wn, wErr
	}

	if err != nil {
		err = &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
		return
	}
	return
}
