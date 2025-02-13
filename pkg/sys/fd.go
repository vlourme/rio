package sys

import (
	"github.com/brickingsoft/errors"
	"golang.org/x/sys/unix"
	"net"
	"os"
	"syscall"
)

func NewFd(family int, sotype int, protocol int) (fd *Fd, err error) {
	sock, sockErr := syscall.Socket(family, sotype|syscall.SOCK_NONBLOCK|syscall.SOCK_CLOEXEC, protocol)
	if sockErr != nil {
		if errors.Is(err, syscall.EPROTONOSUPPORT) || errors.Is(err, syscall.EINVAL) {
			syscall.ForkLock.RLock()
			sock, sockErr = syscall.Socket(family, sotype, protocol)
			if sockErr == nil {
				syscall.CloseOnExec(sock)
			}
			syscall.ForkLock.RUnlock()
			if sockErr != nil {
				err = os.NewSyscallError("socket", err)
				return
			}
			if sockErr = syscall.SetNonblock(sock, true); sockErr != nil {
				_ = syscall.Close(sock)
				err = os.NewSyscallError("setnonblock", err)
				return
			}
		} else {
			err = os.NewSyscallError("socket", err)
			return
		}
	}
	major, minor := KernelVersion()
	if major >= 4 && minor >= 14 {
		_ = syscall.SetsockoptInt(sock, syscall.SOL_SOCKET, unix.SO_ZEROCOPY, 1)
	}
	fd = &Fd{
		sock:   sock,
		family: family,
		sotype: sotype,
		net:    "",
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

func (fd *Fd) RemoteAddr() net.Addr {
	return fd.raddr
}

func (fd *Fd) SetNet(net string) {
	fd.net = net
}

func (fd *Fd) SetLocalAddr(addr net.Addr) {
	fd.laddr = addr
}

func (fd *Fd) SetRemoteAddr(addr net.Addr) {
	fd.raddr = addr
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
		return errors.New("bind failed", errors.WithWrap(saErr))
	}
	if err := syscall.Bind(fd.sock, sa); err != nil {
		return errors.New("bind failed", errors.WithWrap(os.NewSyscallError("bind", err)))
	}
	return nil
}

func (fd *Fd) Close() error {
	return syscall.Close(fd.sock)
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
