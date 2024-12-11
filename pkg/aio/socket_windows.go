//go:build windows

package aio

import (
	"golang.org/x/sys/windows"
	"os"
	"syscall"
)

func sysSocket(family int, sotype int, protocol int) (fd int, err error) {
	wfd, socketErr := windows.WSASocket(int32(family), int32(sotype), int32(protocol), nil, 0, windows.WSA_FLAG_OVERLAPPED|windows.WSA_FLAG_NO_HANDLE_INHERIT)
	if socketErr != nil {
		err = os.NewSyscallError("WSASocket", err)
		return
	}
	fd = int(wfd)
	return
}

func newSocket(family int, sotype int, protocol int, ipv6only bool) (fd int, err error) {
	// socket
	fd, err = sysSocket(family, sotype, protocol)
	// set default opts
	setDefaultSockOptsErr := setDefaultSocketOpts(fd, family, sotype, ipv6only)
	if setDefaultSockOptsErr != nil {
		err = setDefaultSockOptsErr
		_ = windows.Closesocket(windows.Handle(fd))
		return
	}
	return
}

func setDefaultSocketOpts(fd int, family int, sotype int, ipv6only bool) error {
	if family == syscall.AF_INET6 && sotype != syscall.SOCK_RAW {
		err := syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IPV6, syscall.IPV6_V6ONLY, boolint(ipv6only))
		if err != nil {
			return os.NewSyscallError("setsockopt", err)
		}
	}
	if (sotype == syscall.SOCK_DGRAM || sotype == syscall.SOCK_RAW) && family != syscall.AF_UNIX && family != syscall.AF_INET6 {
		err := syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
		if err != nil {
			return os.NewSyscallError("setsockopt", err)
		}
	}
	return nil
}

func setDefaultListenerSocketOpts(_ int) error {
	return nil
}

func setDefaultMulticastSockopts(fd int) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1))
}
