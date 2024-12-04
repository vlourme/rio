//go:build linux

package aio

import (
	"os"
	"syscall"
)

func newSocket(family int, sotype int, protocol int) (fd int, err error) {
	// socket
	fd, err = syscall.Socket(family, sotype|syscall.SOCK_NONBLOCK|syscall.SOCK_CLOEXEC, protocol)
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
	if family == syscall.AF_INET6 && sotype != syscall.SOCK_RAW {
		// set ipv6 only
		err := syscall.SetsockoptInt(fd, syscall.IPPROTO_IPV6, syscall.IPV6_V6ONLY, 1)
		if err != nil {
			return os.NewSyscallError("setsockopt", err)
		}
	}
	if (sotype == syscall.SOCK_DGRAM || sotype == syscall.SOCK_RAW) && family != syscall.AF_UNIX && family != syscall.AF_INET6 {
		// allow broadcast.
		err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
		if err != nil {
			return os.NewSyscallError("setsockopt", err)
		}
	}
	return nil
}

func setDefaultListenerSocketOpts(fd int) error {
	// Allow reuse of recently-used addresses.
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1))
}
