//go:build linux

package aio

import (
	"errors"
	"os"
	"syscall"
)

func sysSocket(family int, sotype int, protocol int) (fd int, err error) {
	fd, err = syscall.Socket(family, sotype|syscall.SOCK_NONBLOCK|syscall.SOCK_CLOEXEC, protocol)
	if err != nil {
		if errors.Is(err, syscall.EPROTONOSUPPORT) || errors.Is(err, syscall.EINVAL) {
			syscall.ForkLock.RLock()
			fd, err = syscall.Socket(family, sotype, protocol)
			if err == nil {
				syscall.CloseOnExec(fd)
			}
			syscall.ForkLock.RUnlock()
			if err != nil {
				err = os.NewSyscallError("socket", err)
				return
			}
			if err = syscall.SetNonblock(fd, true); err != nil {
				_ = syscall.Close(fd)
				err = os.NewSyscallError("setnonblock", err)
				return
			}
		} else {
			err = os.NewSyscallError("socket", err)
			return
		}
	}
	return
}

func newSocket(family int, sotype int, protocol int, ipv6only bool) (fd int, err error) {
	// socket
	fd, err = sysSocket(family, sotype, protocol)
	if err != nil {
		return
	}
	// set default opts
	setDefaultSockOptsErr := setDefaultSocketOpts(fd, family, sotype, ipv6only)
	if setDefaultSockOptsErr != nil {
		err = setDefaultSockOptsErr
		_ = syscall.Close(fd)
		return
	}
	return
}

func setDefaultSocketOpts(fd int, family int, sotype int, ipv6only bool) error {
	if family == syscall.AF_INET6 && sotype != syscall.SOCK_RAW {
		// set ipv6 only
		err := syscall.SetsockoptInt(fd, syscall.IPPROTO_IPV6, syscall.IPV6_V6ONLY, boolint(ipv6only))
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

func setDefaultMulticastSockopts(fd int) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1))
}
