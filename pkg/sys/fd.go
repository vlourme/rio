package sys

import (
	"golang.org/x/sys/unix"
	"net"
	"os"
	"sync/atomic"
	"syscall"
	"time"
)

func NewFd(network string, sock int, family int, sotype int) (fd *Fd) {
	fd = &Fd{
		sock:   sock,
		family: family,
		sotype: sotype,
		net:    network,
		laddr:  nil,
		raddr:  nil,
	}
	return
}

type Fd struct {
	sock   int
	family int
	sotype int
	net    string
	laddr  net.Addr
	raddr  net.Addr
}

func (fd *Fd) Name() string {
	var ls, rs string
	if fd.laddr != nil {
		ls = fd.laddr.String()
	}
	if fd.raddr != nil {
		rs = fd.raddr.String()
	}
	return fd.net + ":" + ls + "->" + rs
}

func (fd *Fd) ZeroReadIsEOF() bool {
	return fd.sotype != syscall.SOCK_DGRAM && fd.sotype != syscall.SOCK_RAW
}

func (fd *Fd) Socket() int {
	return fd.sock
}

func (fd *Fd) Family() int {
	return fd.family
}

func (fd *Fd) SocketType() int {
	return fd.sotype
}

func (fd *Fd) Net() string {
	return fd.net
}

func (fd *Fd) LocalAddr() net.Addr {
	return fd.laddr
}

func (fd *Fd) SetLocalAddr(addr net.Addr) {
	fd.laddr = addr
}

func (fd *Fd) LoadLocalAddr() (err error) {
	sa, saErr := syscall.Getsockname(fd.sock)
	if saErr != nil {
		err = os.NewSyscallError("getsockname", saErr)
		return
	}
	fd.laddr = SockaddrToAddr(fd.net, sa)
	return
}

func (fd *Fd) RemoteAddr() net.Addr {
	return fd.raddr
}

func (fd *Fd) SetRemoteAddr(addr net.Addr) {
	fd.raddr = addr
}

func (fd *Fd) LoadRemoteAddr() (err error) {
	sa, saErr := syscall.Getpeername(fd.sock)
	if saErr != nil {
		err = os.NewSyscallError("getpeername", saErr)
		return
	}
	fd.raddr = SockaddrToAddr(fd.net, sa)
	return
}

func (fd *Fd) SetIpv6only(ipv6only bool) error {
	if fd.family == syscall.AF_INET6 && fd.sotype != syscall.SOCK_RAW {
		// set ipv6 only
		if err := syscall.SetsockoptInt(fd.sock, syscall.IPPROTO_IPV6, syscall.IPV6_V6ONLY, boolint(ipv6only)); err != nil {
			return os.NewSyscallError("setsockopt", err)
		}
	}
	return nil
}

func (fd *Fd) AllowFastOpen(n int) error {
	if n < 1 {
		return nil
	}
	if fd.sotype == syscall.SOCK_STREAM {
		if err := unix.SetsockoptInt(fd.sock, unix.IPPROTO_TCP, unix.TCP_FASTOPEN, n); err != nil {
			return os.NewSyscallError("setsockopt", err)
		}
	}
	return nil
}

func (fd *Fd) AllowBroadcast() error {
	if (fd.sotype == syscall.SOCK_DGRAM || fd.sotype == syscall.SOCK_RAW) && fd.family != syscall.AF_UNIX && fd.family != syscall.AF_INET6 {
		if err := syscall.SetsockoptInt(fd.sock, syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1); err != nil {
			return os.NewSyscallError("setsockopt", err)
		}
	}
	return nil
}

func (fd *Fd) AllowReuseAddr() error {
	if err := os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd.sock, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)); err != nil {
		return os.NewSyscallError("setsockopt", err)
	}
	return nil
}

func (fd *Fd) Bind(addr net.Addr) error {
	sa, saErr := AddrToSockaddr(addr)
	if saErr != nil {
		return saErr
	}
	if err := syscall.Bind(fd.sock, sa); err != nil {
		return os.NewSyscallError("bind", err)
	}
	return nil
}

func (fd *Fd) Close() error {
	return syscall.Close(fd.sock)
}

func (fd *Fd) CloseRead() error {
	return syscall.Shutdown(fd.sock, syscall.SHUT_RD)
}

func (fd *Fd) CloseWrite() error {
	return syscall.Shutdown(fd.sock, syscall.SHUT_WR)
}

func (fd *Fd) SetNoDelay(noDelay bool) error {
	if fd.sotype == syscall.SOCK_STREAM {
		if err := syscall.SetsockoptInt(fd.sock, syscall.IPPROTO_TCP, syscall.TCP_NODELAY, boolint(noDelay)); err != nil {
			return os.NewSyscallError("setsockopt", err)
		}
	}
	return nil
}

func (fd *Fd) SetLinger(sec int) error {
	var l syscall.Linger
	if sec >= 0 {
		l.Onoff = 1
		l.Linger = int32(sec)
	} else {
		l.Onoff = 0
		l.Linger = 0
	}
	if err := syscall.SetsockoptLinger(fd.sock, syscall.SOL_SOCKET, syscall.SO_LINGER, &l); err != nil {
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

func (fd *Fd) SetKeepAlive(keepalive bool) error {
	if err := syscall.SetsockoptInt(fd.sock, syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, boolint(keepalive)); err != nil {
		return os.NewSyscallError("setsockopt", err)
	}
	return nil
}

func (fd *Fd) SetKeepAlivePeriod(d time.Duration) error {
	if d == 0 {
		d = defaultTCPKeepAliveIdle
	} else if d < 0 {
		return nil
	}
	secs := int(roundDurationUp(d, time.Second))
	if err := syscall.SetsockoptInt(fd.sock, syscall.IPPROTO_TCP, syscall.TCP_KEEPIDLE, secs); err != nil {
		return os.NewSyscallError("setsockopt", err)
	}
	return nil
}

func (fd *Fd) SetKeepAliveInterval(d time.Duration) error {
	if d == 0 {
		d = defaultTCPKeepAliveInterval
	} else if d < 0 {
		return nil
	}
	secs := int(roundDurationUp(d, time.Second))
	if err := syscall.SetsockoptInt(fd.sock, syscall.IPPROTO_TCP, syscall.TCP_KEEPINTVL, secs); err != nil {
		return os.NewSyscallError("setsockopt", err)
	}
	return nil
}

func (fd *Fd) SetKeepAliveCount(n int) error {
	if n == 0 {
		n = defaultTCPKeepAliveCount
	} else if n < 0 {
		return nil
	}
	if err := syscall.SetsockoptInt(fd.sock, syscall.IPPROTO_TCP, syscall.TCP_KEEPCNT, n); err != nil {
		return os.NewSyscallError("setsockopt", err)
	}
	return nil
}

func (fd *Fd) SetKeepAliveConfig(config net.KeepAliveConfig) error {
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

func (fd *Fd) Dup() (int, string, error) {
	return DupCloseOnExec(fd.sock)
}

func (fd *Fd) ctrlNetwork() string {
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

var dupCloexecUnsupported atomic.Bool

func DupCloseOnExec(fd int) (int, string, error) {
	if syscall.F_DUPFD_CLOEXEC != 0 && !dupCloexecUnsupported.Load() {
		r0, err := Fcntl(fd, syscall.F_DUPFD_CLOEXEC, 0)
		if err == nil {
			return r0, "", nil
		}
		switch err {
		case syscall.EINVAL, syscall.ENOSYS:
			// Old kernel, or js/wasm (which returns
			// ENOSYS). Fall back to the portable way from
			// now on.
			dupCloexecUnsupported.Store(true)
		default:
			return -1, "fcntl", err
		}
	}
	return dupCloseOnExecOld(fd)
}

func dupCloseOnExecOld(fd int) (int, string, error) {
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	newfd, err := syscall.Dup(fd)
	if err != nil {
		return -1, "dup", err
	}
	syscall.CloseOnExec(newfd)
	return newfd, "", nil
}

func Fcntl(fd int, cmd int, arg int) (int, error) {
	val, errno := fcntl(int32(fd), int32(cmd), int32(arg))
	if val == -1 {
		return int(val), syscall.Errno(errno)
	}
	return int(val), nil
}

func fcntl(fd, cmd, arg int32) (ret int32, errno int32) {
	r, _, err := syscall.Syscall6(syscall.SYS_FCNTL, uintptr(fd), uintptr(cmd), uintptr(arg), 0, 0, 0)
	return int32(r), int32(err)
}

func (fd *Fd) SetIPv4MulticastInterface(ifi *net.Interface) error {
	ip, err := interfaceToIPv4Addr(ifi)
	if err != nil {
		return err
	}
	var a [4]byte
	copy(a[:], ip.To4())
	return syscall.SetsockoptInet4Addr(fd.sock, syscall.IPPROTO_IP, syscall.IP_MULTICAST_IF, a)
}

func (fd *Fd) SetIPv4MulticastLoopback(ok bool) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd.sock, syscall.IPPROTO_IP, syscall.IP_MULTICAST_LOOP, boolint(ok)))
}

func (fd *Fd) JoinIPv4Group(ifi *net.Interface, ip net.IP) error {
	mreq := &syscall.IPMreq{Multiaddr: [4]byte{ip[0], ip[1], ip[2], ip[3]}}
	if err := setIPv4MreqToInterface(mreq, ifi); err != nil {
		return err
	}
	return os.NewSyscallError("setsockopt", syscall.SetsockoptIPMreq(fd.sock, syscall.IPPROTO_IP, syscall.IP_ADD_MEMBERSHIP, mreq))
}

func (fd *Fd) SetIPv6MulticastInterface(ifi *net.Interface) error {
	var v int
	if ifi != nil {
		v = ifi.Index
	}
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd.sock, syscall.IPPROTO_IPV6, syscall.IPV6_MULTICAST_IF, v))
}

func (fd *Fd) SetIPv6MulticastLoopback(ok bool) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd.sock, syscall.IPPROTO_IPV6, syscall.IPV6_MULTICAST_LOOP, boolint(ok)))
}

func (fd *Fd) JoinIPv6Group(ifi *net.Interface, ip net.IP) error {
	mreq := &syscall.IPv6Mreq{}
	copy(mreq.Multiaddr[:], ip)
	if ifi != nil {
		mreq.Interface = uint32(ifi.Index)
	}
	return os.NewSyscallError("setsockopt", syscall.SetsockoptIPv6Mreq(fd.sock, syscall.IPPROTO_IPV6, syscall.IPV6_JOIN_GROUP, mreq))
}
