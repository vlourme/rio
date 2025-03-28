//go:build linux

package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"net"
	"os"
	"reflect"
	"runtime"
	"strings"
	"syscall"
)

func Listen(ctx context.Context, vortex *Vortex, network string, proto int, addr net.Addr, reusePort bool, control sys.ControlContextFn) (fd *NetFd, err error) {
	// addr
	if addr != nil && reflect.ValueOf(addr).IsNil() {
		addr = nil
	}
	if addr == nil {
		err = errors.New("missing address")
		return
	}
	// network
	tcpDeferAccept := false
	sotype := 0
	addrPort := 0
	switch network {
	case "tcp", "tcp4", "tcp6":
		sotype = syscall.SOCK_STREAM
		tcpDeferAccept = true
		tcpAddr := addr.(*net.TCPAddr)
		addrPort = tcpAddr.Port
		break
	case "unix":
		sotype = syscall.SOCK_STREAM
	case "unixpacket":
		sotype = syscall.SOCK_SEQPACKET
		break
	default:
		err = errors.New("unsupported network")
		return
	}

	// family
	family, ipv6only := sys.FavoriteAddrFamily(network, addr, nil, "listen")
	// sock
	var (
		regular = -1
		direct  = -1
	)
	if vortex.DirectAllocEnabled() {
		op := vortex.acquireOperation()
		op.WithDirect(true).PrepareSocket(family, sotype|syscall.SOCK_NONBLOCK, proto)
		direct, _, err = vortex.submitAndWait(op)
		vortex.releaseOperation(op)
	} else {
		regular, err = sys.NewSocket(family, sotype, proto)
	}
	if err != nil {
		return
	}
	// fd
	fd = &NetFd{
		Fd: Fd{
			regular:       regular,
			direct:        direct,
			isStream:      sotype == syscall.SOCK_STREAM,
			zeroReadIsEOF: sotype != syscall.SOCK_DGRAM && sotype != syscall.SOCK_RAW,
			vortex:        vortex,
		},
		family: family,
		sotype: sotype,
		net:    network,
		laddr:  addr,
		raddr:  nil,
	}
	// ipv6
	if ipv6only {
		if err = fd.SetIpv6only(true); err != nil {
			_ = fd.Close()
			return
		}
	}
	// zero copy
	if err = fd.SetZeroCopy(true); err != nil {
		_ = fd.Close()
		return
	}
	// reuse addr
	if err = fd.SetReuseAddr(true); err != nil {
		_ = fd.Close()
		return
	}
	// tcp defer accept
	if tcpDeferAccept {
		if err = fd.SetTCPDeferAccept(true); err != nil {
			_ = fd.Close()
			return
		}
	}
	// reuse port
	if reusePort && addrPort > 0 {
		if err = fd.SetReusePort(addrPort); err != nil {
			_ = fd.Close()
			return
		}
		if err = fd.SetCBPF(runtime.NumCPU()); err != nil {
			_ = fd.Close()
			return
		}
	}
	// control
	if control != nil {
		if regular == -1 {
			if regular, err = vortex.FixedFdInstall(direct); err == nil {
				_ = fd.Close()
				return
			}
			fd.regular = regular
		}
		raw, rawErr := fd.SyscallConn()
		if rawErr != nil {
			_ = fd.Close()
			err = rawErr
			return
		}
		if err = control(ctx, fd.CtrlNetwork(), addr.String(), raw); err != nil {
			_ = fd.Close()
			return
		}
	}
	// bind
	if err = fd.Bind(addr); err != nil {
		_ = fd.Close()
		return
	}
	// listen
	backlog := sys.MaxListenerBacklog()
	if liburing.VersionEnable(6, 11, 0) && fd.Registered() {
		op := fd.vortex.acquireOperation()
		op.PrepareListen(fd, backlog)
		_, _, err = fd.vortex.submitAndWait(op)
		fd.vortex.releaseOperation(op)
		if err != nil {
			_ = fd.Close()
			return
		}
	} else {
		if !fd.Installed() {
			if err = fd.Install(); err != nil {
				_ = fd.Close()
				return
			}
		}
		if err = syscall.Listen(fd.regular, backlog); err != nil {
			_ = fd.Close()
			err = os.NewSyscallError("listen", err)
			return
		}
	}
	return
}

func ListenPacket(ctx context.Context, vortex *Vortex, network string, proto int, addr net.Addr, ifi *net.Interface, control sys.ControlContextFn) (fd *NetFd, err error) {
	// addr
	if addr != nil && reflect.ValueOf(addr).IsNil() {
		addr = nil
	}
	if addr == nil {
		err = errors.New("missing address")
		return
	}
	// network
	sotype := 0
	switch network {
	case "udp", "udp4", "udp6":
		sotype = syscall.SOCK_DGRAM
		udpAddr := addr.(*net.UDPAddr)
		if udpAddr.IP != nil && udpAddr.IP.IsMulticast() {
			localUdpAddr := *udpAddr
			if strings.HasSuffix(network, "6") {
				localUdpAddr.IP = net.IPv6zero
			} else {
				localUdpAddr.IP = net.IPv4zero.To4()
			}
			addr = &localUdpAddr
		}
		break
	case "unixgram":
		sotype = syscall.SOCK_DGRAM
		break
	default:
		err = errors.New("unsupported network")
		return
	}

	// family
	family, ipv6only := sys.FavoriteAddrFamily(network, addr, nil, "listen")
	// sock
	var (
		regular = -1
		direct  = -1
	)
	if vortex.DirectAllocEnabled() {
		op := vortex.acquireOperation()
		op.WithDirect(true).PrepareSocket(family, sotype|syscall.SOCK_NONBLOCK, proto)
		direct, _, err = vortex.submitAndWait(op)
		vortex.releaseOperation(op)
	} else {
		regular, err = sys.NewSocket(family, sotype, proto)
	}
	if err != nil {
		return
	}
	// fd
	fd = &NetFd{
		Fd: Fd{
			regular:       regular,
			direct:        direct,
			isStream:      sotype == syscall.SOCK_STREAM,
			zeroReadIsEOF: sotype != syscall.SOCK_DGRAM && sotype != syscall.SOCK_RAW,
			vortex:        vortex,
		},
		family: family,
		sotype: sotype,
		net:    network,
		laddr:  addr,
		raddr:  nil,
	}
	// ipv6
	if ipv6only {
		if err = fd.SetIpv6only(true); err != nil {
			_ = fd.Close()
			return
		}
	}
	// zero copy
	if err = fd.SetZeroCopy(true); err != nil {
		_ = fd.Close()
		return
	}
	// reuse addr
	if err = fd.SetReuseAddr(true); err != nil {
		_ = fd.Close()
		return
	}
	//  broadcast
	if err = fd.SetBroadcast(true); err != nil {
		_ = fd.Close()
		return
	}
	// multicast
	if ifi != nil {
		udpAddr, ok := addr.(*net.UDPAddr)
		if !ok {
			_ = fd.Close()
			err = errors.New("has ifi but addr is not udp addr")
			return
		}
		if ip4 := udpAddr.IP.To4(); ip4 != nil {
			if err = fd.SetIPv4MulticastInterface(ifi); err != nil {
				_ = fd.Close()
				return
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
			if err = fd.SetIPv6MulticastInterface(ifi); err != nil {
				_ = fd.Close()
				return
			}
			if err = fd.SetIPv6MulticastLoopback(false); err != nil {
				_ = fd.Close()
				return
			}
			if err = fd.JoinIPv6Group(ifi, udpAddr.IP); err != nil {
				_ = fd.Close()
				return
			}
		}
	}
	// control
	if control != nil {
		if regular == -1 {
			if regular, err = vortex.FixedFdInstall(direct); err == nil {
				_ = fd.Close()
				return
			}
			fd.regular = regular
		}
		raw, rawErr := fd.SyscallConn()
		if rawErr != nil {
			_ = fd.Close()
			err = rawErr
			return
		}
		if err = control(ctx, fd.CtrlNetwork(), addr.String(), raw); err != nil {
			_ = fd.Close()
			return
		}
	}
	// bind
	if err = fd.Bind(addr); err != nil {
		_ = fd.Close()
		return
	}
	return
}
