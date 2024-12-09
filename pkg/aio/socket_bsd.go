//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

import (
	"os"
	"runtime"
	"syscall"
)

func newSocket(family int, sotype int, protocol int) (fd int, err error) {
	// socket
	fd, err = syscall.Socket(family, sotype, protocol)
	if err != nil {
		err = os.NewSyscallError("socket", err)
		return
	}
	// set default opts
	setDefaultSockOptsErr := setDefaultSocketOpts(fd, family, sotype)
	if setDefaultSockOptsErr != nil {
		err = setDefaultSockOptsErr
		_ = syscall.Close(fd)
		return
	}
	return
}

func setDefaultSocketOpts(fd int, family int, sotype int) error {
	if runtime.GOOS == "dragonfly" && sotype != syscall.SOCK_RAW {
		switch family {
		case syscall.AF_INET:
			_ = syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_PORTRANGE, syscall.IP_PORTRANGE_HIGH)
		case syscall.AF_INET6:
			_ = syscall.SetsockoptInt(fd, syscall.IPPROTO_IPV6, syscall.IPV6_PORTRANGE, syscall.IPV6_PORTRANGE_HIGH)
		}
	}
	if family == syscall.AF_INET6 && sotype != syscall.SOCK_RAW && runtime.GOOS != "dragonfly" && runtime.GOOS != "openbsd" {
		_ = syscall.SetsockoptInt(fd, syscall.IPPROTO_IPV6, syscall.IPV6_V6ONLY, 1)
	}
	if (sotype == syscall.SOCK_DGRAM || sotype == syscall.SOCK_RAW) && family != syscall.AF_UNIX {
		return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1))
	}
	return nil
}

func setDefaultListenerSocketOpts(fd int) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1))
}

func setDefaultMulticastSockopts(s int) error {
	if err := syscall.SetsockoptInt(s, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
		return os.NewSyscallError("setsockopt", err)
	}
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(s, syscall.SOL_SOCKET, syscall.SO_REUSEPORT, 1))
}
