//go:build linux

package aio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring/aio/sys"
	"golang.org/x/sys/unix"
	"net"
	"os"
	"reflect"
	"syscall"
	"time"
)

type OpenNetFdMode string

const (
	ListenMode OpenNetFdMode = "listen"
	DialMode   OpenNetFdMode = "dial"
)

func OpenNetFd(
	ctx context.Context, vortex *Vortex,
	mode OpenNetFdMode,
	network string, sotype int, proto int,
	laddr net.Addr, raddr net.Addr,
	directAlloc bool,
) (fd *NetFd, err error) {
	if laddr != nil && reflect.ValueOf(laddr).IsNil() {
		laddr = nil
	}
	if raddr != nil && reflect.ValueOf(raddr).IsNil() {
		raddr = nil
	}
	if laddr == nil && raddr == nil {
		err = errors.New("missing address")
		return
	}
	// sock
	family, ipv6only := sys.FavoriteAddrFamily(network, laddr, raddr, string(mode))
	var (
		regular = -1
		direct  = -1
		sockErr error
	)
	if directAlloc {
		op := vortex.acquireOperation()
		op.WithDirect(true).PrepareSocket(family, sotype, proto)
		direct, _, sockErr = vortex.submitAndWait(ctx, op)
		vortex.releaseOperation(op)
		if sockErr == nil {
			regular, sockErr = vortex.FixedFdInstall(ctx, direct)
		}
	} else {
		regular, sockErr = sys.NewSocket(family, sotype, proto)
	}
	if sockErr != nil {
		err = sockErr
		return
	}
	// fd
	cc, cancel := context.WithCancel(ctx)
	fd = &NetFd{
		ctx:       cc,
		cancel:    cancel,
		regular:   regular,
		direct:    direct,
		allocated: directAlloc,
		family:    family,
		sotype:    sotype,
		net:       network,
		async:     false,
		laddr:     laddr,
		raddr:     raddr,
		vortex:    vortex,
	}
	// ipv6
	if ipv6only {
		if err = fd.SetIpv6only(true); err != nil {
			_ = fd.Close()
			return
		}
	}
	return
}

func newAcceptedNetFd(ln *NetFd, accepted int, directAllocated bool) (fd *NetFd, err error) {
	ctx, cancel := context.WithCancel(ln.ctx)
	vortex := ln.vortex
	fd = &NetFd{
		ctx:       ctx,
		cancel:    cancel,
		regular:   -1,
		direct:    -1,
		allocated: directAllocated,
		family:    ln.family,
		sotype:    ln.sotype,
		net:       ln.net,
		async:     ln.async,
		laddr:     nil,
		raddr:     nil,
		vortex:    vortex,
	}
	if directAllocated {
		fd.direct = accepted
		regular, installErr := vortex.FixedFdInstall(ctx, fd.direct)
		if installErr != nil {
			_ = fd.Close()
			err = installErr
			return
		}
		fd.regular = regular
		//fd.SetCloseOnExec()
	} else {
		fd.regular = accepted
	}
	return
}

type NetFd struct {
	ctx       context.Context
	cancel    context.CancelFunc
	regular   int
	direct    int
	allocated bool
	family    int
	sotype    int
	net       string
	async     bool
	laddr     net.Addr
	raddr     net.Addr
	vortex    *Vortex
}

func (fd *NetFd) Name() string {
	var ls, rs string
	if fd.laddr != nil {
		ls = fd.laddr.String()
	}
	if fd.raddr != nil {
		rs = fd.raddr.String()
	}
	return fd.net + ":" + ls + "->" + rs
}

func (fd *NetFd) Context() context.Context {
	return fd.ctx
}

func (fd *NetFd) Async() bool {
	return fd.async
}

func (fd *NetFd) SetAsync(ok bool) {
	fd.async = ok
}

func (fd *NetFd) Vortex() *Vortex {
	return fd.vortex
}

func (fd *NetFd) ZeroReadIsEOF() bool {
	return fd.sotype != syscall.SOCK_DGRAM && fd.sotype != syscall.SOCK_RAW
}

func (fd *NetFd) Socket() (sock int, direct bool) {
	if fd.Registered() {
		sock = fd.direct
		direct = true
		return
	}
	return fd.regular, false
}

func (fd *NetFd) RegularSocket() int {
	return fd.regular
}

func (fd *NetFd) DirectSocket() int {
	return fd.direct
}

func (fd *NetFd) Family() int {
	return fd.family
}

func (fd *NetFd) SocketType() int {
	return fd.sotype
}

func (fd *NetFd) Net() string {
	return fd.net
}

func (fd *NetFd) LocalAddr() net.Addr {
	if fd.laddr == nil {
		if fd.regular == -1 && fd.direct != -1 {
			regular, installErr := fd.vortex.FixedFdInstall(fd.ctx, fd.direct)
			if installErr != nil {
				return nil
			}
			fd.regular = regular
		}
		sa, saErr := syscall.Getsockname(fd.regular)
		if saErr != nil {
			return nil
		}
		fd.laddr = sys.SockaddrToAddr(fd.net, sa)
	}
	return fd.laddr
}

func (fd *NetFd) SetLocalAddr(addr net.Addr) {
	fd.laddr = addr
}

func (fd *NetFd) RemoteAddr() net.Addr {
	if fd.raddr == nil {
		if fd.regular == -1 && fd.direct != -1 {
			regular, installErr := fd.vortex.FixedFdInstall(fd.ctx, fd.direct)
			if installErr != nil {
				return nil
			}
			fd.regular = regular
		}
		sa, saErr := syscall.Getpeername(fd.regular)
		if saErr != nil {
			return nil
		}
		fd.raddr = sys.SockaddrToAddr(fd.net, sa)
	}
	return fd.raddr
}

func (fd *NetFd) SetRemoteAddr(addr net.Addr) {
	fd.raddr = addr
}

func (fd *NetFd) Bind(addr net.Addr) error {
	sa, saErr := sys.AddrToSockaddr(addr)
	if saErr != nil {
		return saErr
	}
	if err := syscall.Bind(fd.regular, sa); err != nil {
		return os.NewSyscallError("bind", err)
	}
	return nil
}

func (fd *NetFd) ReadBuffer() (n int, err error) {
	n, err = syscall.GetsockoptInt(fd.regular, syscall.SOL_SOCKET, syscall.SO_RCVBUF)
	if err != nil {
		err = os.NewSyscallError("getsockopt", err)
		return
	}
	return
}

func (fd *NetFd) SetReadBuffer(bytes int) error {
	if err := syscall.SetsockoptInt(fd.regular, syscall.SOL_SOCKET, syscall.SO_RCVBUF, bytes); err != nil {
		return os.NewSyscallError("setsockopt", err)
	}
	return nil
}

func (fd *NetFd) WriteBuffer() (n int, err error) {
	n, err = syscall.GetsockoptInt(fd.regular, syscall.SOL_SOCKET, syscall.SO_SNDBUF)
	if err != nil {
		err = os.NewSyscallError("getsockopt", err)
		return
	}
	return
}

func (fd *NetFd) SetWriteBuffer(bytes int) error {
	if err := syscall.SetsockoptInt(fd.regular, syscall.SOL_SOCKET, syscall.SO_SNDBUF, bytes); err != nil {
		return os.NewSyscallError("setsockopt", err)
	}
	return nil
}

func (fd *NetFd) SetNoDelay(noDelay bool) error {
	if fd.sotype == syscall.SOCK_STREAM {
		if err := syscall.SetsockoptInt(fd.regular, syscall.IPPROTO_TCP, syscall.TCP_NODELAY, boolint(noDelay)); err != nil {
			return os.NewSyscallError("setsockopt", err)
		}
	}
	return nil
}

func (fd *NetFd) SetLinger(sec int) error {
	var l syscall.Linger
	if sec >= 0 {
		l.Onoff = 1
		l.Linger = int32(sec)
	} else {
		l.Onoff = 0
		l.Linger = 0
	}
	if err := syscall.SetsockoptLinger(fd.regular, syscall.SOL_SOCKET, syscall.SO_LINGER, &l); err != nil {
		return os.NewSyscallError("setsockopt", err)
	}
	return nil
}

const (
	// defaultTCPKeepAliveIdle is a default constant value for TCP_KEEPIDLE.
	// See go.dev/issue/31510 for details.
	defaultTCPKeepAliveIdle = 15 * time.Second

	// defaultTCPKeepAliveInterval is a default constant value for TCP_KEEPINTVL.
	// It is the same as defaultTCPKeepAliveIdle, see go.dev/issue/31510 for details.
	defaultTCPKeepAliveInterval = 15 * time.Second

	// defaultTCPKeepAliveCount is a default constant value for TCP_KEEPCNT.
	defaultTCPKeepAliveCount = 9
)

func roundDurationUp(d time.Duration, to time.Duration) time.Duration {
	return (d + to - 1) / to
}

func (fd *NetFd) SetKeepAlive(keepalive bool) error {

	if err := syscall.SetsockoptInt(fd.regular, syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, boolint(keepalive)); err != nil {
		return os.NewSyscallError("setsockopt", err)
	}
	return nil
}

func (fd *NetFd) SetKeepAlivePeriod(d time.Duration) error {
	if d == 0 {
		d = defaultTCPKeepAliveIdle
	} else if d < 0 {
		return nil
	}
	secs := int(roundDurationUp(d, time.Second))
	if err := syscall.SetsockoptInt(fd.regular, syscall.IPPROTO_TCP, syscall.TCP_KEEPIDLE, secs); err != nil {
		return os.NewSyscallError("setsockopt", err)
	}
	return nil
}

func (fd *NetFd) SetKeepAliveInterval(d time.Duration) error {
	if d == 0 {
		d = defaultTCPKeepAliveInterval
	} else if d < 0 {
		return nil
	}
	secs := int(roundDurationUp(d, time.Second))
	if err := syscall.SetsockoptInt(fd.regular, syscall.IPPROTO_TCP, syscall.TCP_KEEPINTVL, secs); err != nil {
		return os.NewSyscallError("setsockopt", err)
	}
	return nil
}

func (fd *NetFd) SetKeepAliveCount(n int) error {
	if n == 0 {
		n = defaultTCPKeepAliveCount
	} else if n < 0 {
		return nil
	}
	if err := syscall.SetsockoptInt(fd.regular, syscall.IPPROTO_TCP, syscall.TCP_KEEPCNT, n); err != nil {
		return os.NewSyscallError("setsockopt", err)
	}
	return nil
}

func (fd *NetFd) SetKeepAliveConfig(config net.KeepAliveConfig) error {
	if err := fd.SetKeepAlive(config.Enable); err != nil {
		return err
	}
	if err := fd.SetKeepAlivePeriod(config.Idle); err != nil {
		return err
	}
	if err := fd.SetKeepAliveInterval(config.Interval); err != nil {
		return err
	}
	if err := fd.SetKeepAliveCount(config.Count); err != nil {
		return err
	}
	return nil
}

func (fd *NetFd) SetIpv6only(ipv6only bool) error {
	if fd.family == syscall.AF_INET6 && fd.sotype != syscall.SOCK_RAW {
		if err := syscall.SetsockoptInt(fd.regular, syscall.IPPROTO_IPV6, syscall.IPV6_V6ONLY, boolint(ipv6only)); err != nil {
			return os.NewSyscallError("setsockopt", err)
		}
	}
	return nil
}

func (fd *NetFd) SetNonblocking(nonblocking bool) error {
	if err := syscall.SetNonblock(fd.regular, nonblocking); err != nil {
		return os.NewSyscallError("setnonblock", err)
	}
	return nil
}

func (fd *NetFd) Nonblocking() (ok bool, err error) {
	flag, getErr := sys.Fcntl(fd.regular, syscall.F_GETFL, 0)
	if getErr != nil {
		err = os.NewSyscallError("fcntl", getErr)
		return
	}
	ok = flag&syscall.O_NONBLOCK != 0
	return
}

func (fd *NetFd) SetCloseOnExec() {
	syscall.CloseOnExec(fd.regular)
}

func (fd *NetFd) SetBroadcast(ok bool) error {
	if (fd.sotype == syscall.SOCK_DGRAM || fd.sotype == syscall.SOCK_RAW) && fd.family != syscall.AF_UNIX && fd.family != syscall.AF_INET6 {
		if err := syscall.SetsockoptInt(fd.regular, syscall.SOL_SOCKET, syscall.SO_BROADCAST, boolint(ok)); err != nil {
			return os.NewSyscallError("setsockopt", err)
		}
	}
	return nil
}

func (fd *NetFd) SetReuseAddr(ok bool) error {
	if err := os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd.regular, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, boolint(ok))); err != nil {
		return os.NewSyscallError("setsockopt", err)
	}
	return nil
}

func (fd *NetFd) SetReusePort(reusePort int) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd.regular, syscall.SOL_SOCKET, unix.SO_REUSEPORT, reusePort))
}

func (fd *NetFd) SetIPv4MulticastInterface(ifi *net.Interface) error {
	ip, err := sys.InterfaceToIPv4Addr(ifi)
	if err != nil {
		return err
	}
	var a [4]byte
	copy(a[:], ip.To4())
	return syscall.SetsockoptInet4Addr(fd.regular, syscall.IPPROTO_IP, syscall.IP_MULTICAST_IF, a)
}

func (fd *NetFd) SetIPv4MulticastLoopback(ok bool) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd.regular, syscall.IPPROTO_IP, syscall.IP_MULTICAST_LOOP, boolint(ok)))
}

func (fd *NetFd) JoinIPv4Group(ifi *net.Interface, ip net.IP) error {
	mreq := &syscall.IPMreq{Multiaddr: [4]byte{ip[0], ip[1], ip[2], ip[3]}}
	if err := sys.SetIPv4MreqToInterface(mreq, ifi); err != nil {
		return err
	}
	return os.NewSyscallError("setsockopt", syscall.SetsockoptIPMreq(fd.regular, syscall.IPPROTO_IP, syscall.IP_ADD_MEMBERSHIP, mreq))
}

func (fd *NetFd) SetIPv6MulticastInterface(ifi *net.Interface) error {
	var v int
	if ifi != nil {
		v = ifi.Index
	}
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd.regular, syscall.IPPROTO_IPV6, syscall.IPV6_MULTICAST_IF, v))
}

func (fd *NetFd) SetIPv6MulticastLoopback(ok bool) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd.regular, syscall.IPPROTO_IPV6, syscall.IPV6_MULTICAST_LOOP, boolint(ok)))
}

func (fd *NetFd) JoinIPv6Group(ifi *net.Interface, ip net.IP) error {
	mreq := &syscall.IPv6Mreq{}
	copy(mreq.Multiaddr[:], ip)
	if ifi != nil {
		mreq.Interface = uint32(ifi.Index)
	}
	return os.NewSyscallError("setsockopt", syscall.SetsockoptIPv6Mreq(fd.regular, syscall.IPPROTO_IPV6, syscall.IPV6_JOIN_GROUP, mreq))
}

func (fd *NetFd) Dup() (int, string, error) {
	return sys.DupCloseOnExec(fd.regular)
}

func (fd *NetFd) CtrlNetwork() string {
	switch fd.net {
	case "unix", "unixgram", "unixpacket":
		return fd.net
	}
	switch fd.net[len(fd.net)-1] {
	case '4', '6':
		return fd.net
	}
	if fd.family == syscall.AF_INET {
		return fd.net + "4"
	}
	return fd.net + "6"
}

func boolint(b bool) int {
	if b {
		return 1
	}
	return 0
}
