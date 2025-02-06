//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

import (
	"github.com/brickingsoft/errors"
	"net"
	"os"
	"syscall"
)

func connect(network string, family int, sotype int, proto int, ipv6only bool, raddr net.Addr, laddr net.Addr, cb OperationCallback) {
	// create sock
	sock, sockErr := newSocket(family, sotype, proto, ipv6only)
	if sockErr != nil {
		err := errors.New(
			"connect failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpConnect),
			errors.WithWrap(sockErr),
		)
		cb(Userdata{}, err)
		return
	}
	// try set SO_BROADCAST
	if sotype == syscall.SOCK_DGRAM && (family == syscall.AF_INET || family == syscall.AF_INET6) {
		setBroadcastErr := syscall.SetsockoptInt(sock, syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
		if setBroadcastErr != nil {
			_ = syscall.Close(sock)
			err := errors.New(
				"connect failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithMeta(errMetaOpKey, errMetaOpConnect),
				errors.WithWrap(os.NewSyscallError("setsockopt", setBroadcastErr)),
			)
			cb(Userdata{}, err)
			return
		}
	}

	// net fd
	conn := newNetFd(sock, network, family, sotype, proto, ipv6only, nil, nil)

	// local addr
	if laddr != nil {
		lsa := AddrToSockaddr(laddr)
		bindErr := syscall.Bind(sock, lsa)
		if bindErr != nil {
			_ = syscall.Close(sock)
			err := errors.New(
				"connect failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithMeta(errMetaOpKey, errMetaOpConnect),
				errors.WithWrap(os.NewSyscallError("bind", bindErr)),
			)
			cb(Userdata{}, err)
			return
		}
		conn.localAddr = laddr
		if raddr == nil {
			cb(Userdata{Fd: conn}, nil)
			return
		}
	}
	// remote addr
	if raddr == nil {
		_ = syscall.Close(sock)
		err := errors.New(
			"connect failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpConnect),
			errors.WithWrap(syscall.Errno(22)),
		)
		cb(Userdata{}, err)
		return
	}
	sa := AddrToSockaddr(raddr)

	// connect
	for {
		if connectErr := syscall.Connect(sock, sa); connectErr != nil {
			if errors.Is(connectErr, syscall.EINPROGRESS) || errors.Is(connectErr, syscall.EALREADY) || errors.Is(connectErr, syscall.EINTR) {
				break
			}
			if errors.Is(connectErr, syscall.EAGAIN) {
				continue
			}
			_ = syscall.Close(sock)
			err := errors.New(
				"connect failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithMeta(errMetaOpKey, errMetaOpConnect),
				errors.WithWrap(os.NewSyscallError("connect", connectErr)),
			)
			cb(Userdata{}, err)
			return
		}
		break
	}
	// local addr
	if conn.localAddr == nil {
		lsa, lsaErr := syscall.Getsockname(sock)
		if lsaErr != nil {
			_ = syscall.Close(sock)
			err := errors.New(
				"connect failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithMeta(errMetaOpKey, errMetaOpConnect),
				errors.WithWrap(os.NewSyscallError("getsockname", lsaErr)),
			)
			cb(Userdata{}, err)
			return
		}
		la := SockaddrToAddr(network, lsa)
		conn.localAddr = la
	}
	// cb
	userdata := Userdata{
		Fd: conn,
	}
	cb(userdata, nil)
	return
}
