//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

import (
	"errors"
	"net"
	"os"
	"syscall"
)

func connect(network string, family int, sotype int, proto int, ipv6only bool, raddr net.Addr, laddr net.Addr, cb OperationCallback) {
	// create sock
	sock, sockErr := newSocket(family, sotype, proto, ipv6only)
	if sockErr != nil {
		cb(0, Userdata{}, sockErr)
		return
	}
	// try set SO_BROADCAST
	if sotype == syscall.SOCK_DGRAM && (family == syscall.AF_INET || family == syscall.AF_INET6) {
		setBroadcastErr := syscall.SetsockoptInt(sock, syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
		if setBroadcastErr != nil {
			_ = syscall.Close(sock)
			cb(0, Userdata{}, os.NewSyscallError("setsockopt", setBroadcastErr))
			return
		}
	}

	// net fd
	nfd := &netFd{
		handle:     sock,
		network:    network,
		family:     family,
		socketType: sock,
		protocol:   proto,
		ipv6only:   ipv6only,
		localAddr:  nil,
		remoteAddr: nil,
		rop:        Operator{},
		wop:        Operator{},
	}
	nfd.rop.fd = nfd
	nfd.wop.fd = nfd

	// local addr
	if laddr != nil {
		lsa := AddrToSockaddr(laddr)
		bindErr := syscall.Bind(sock, lsa)
		if bindErr != nil {
			_ = syscall.Close(sock)
			cb(0, Userdata{}, os.NewSyscallError("bind", bindErr))
			return
		}
		nfd.localAddr = laddr
		if raddr == nil {
			userdata := Userdata{
				Fd: nfd,
			}
			cb(sock, userdata, nil)
			return
		}
	}
	// remote addr
	if raddr == nil {
		_ = syscall.Close(sock)
		cb(0, Userdata{}, syscall.Errno(22))
		return
	}
	sa := AddrToSockaddr(raddr)

	// connect
	for {
		if connectErr := syscall.Connect(sock, sa); connectErr != nil {
			if errors.Is(connectErr, syscall.EAGAIN) || errors.Is(connectErr, syscall.EINTR) || errors.Is(connectErr, syscall.ECONNABORTED) {
				continue
			}
			cb(0, Userdata{}, os.NewSyscallError("connect", connectErr))
			return
		}
		break
	}
	// local addr
	if nfd.localAddr == nil {
		lsa, lsaErr := syscall.Getsockname(sock)
		if lsaErr != nil {
			_ = syscall.Close(sock)
			cb(0, Userdata{}, os.NewSyscallError("getsockname", lsaErr))
			return
		}
		la := SockaddrToAddr(network, lsa)
		nfd.localAddr = la
	}
	// cb
	userdata := Userdata{
		Fd: nfd,
	}
	cb(0, userdata, nil)
	return
}
