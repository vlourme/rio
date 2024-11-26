//go:build windows

package aio

import (
	"golang.org/x/sys/windows"
	"os"
	"syscall"
)

func newSocket(family int, sotype int, protocol int) (fd int, err error) {
	// socket
	wfd, socketErr := windows.WSASocket(int32(family), int32(sotype), int32(protocol), nil, 0, windows.WSA_FLAG_OVERLAPPED|windows.WSA_FLAG_NO_HANDLE_INHERIT)
	if socketErr != nil {
		err = os.NewSyscallError("WSASocket", err)
		return
	}
	fd = int(wfd)
	// set default opts
	setDefaultSockOptsErr := setDefaultSocketOpts(fd, family, sotype)
	if setDefaultSockOptsErr != nil {
		err = setDefaultSockOptsErr
		_ = windows.Closesocket(wfd)
		return
	}
	return
}

func setDefaultSocketOpts(fd int, family int, sotype int) error {
	if family == syscall.AF_INET6 && sotype != syscall.SOCK_RAW {
		// set ipv6 only
		err := syscall.SetsockoptInt(syscall.Handle(fd), syscall.IPPROTO_IPV6, syscall.IPV6_V6ONLY, 1)
		if err != nil {
			return os.NewSyscallError("setsockopt", err)
		}
	}
	if (sotype == syscall.SOCK_DGRAM || sotype == syscall.SOCK_RAW) && family != syscall.AF_UNIX && family != syscall.AF_INET6 {
		// allow broadcast.
		err := syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
		if err != nil {
			return os.NewSyscallError("setsockopt", err)
		}
	}
	return nil
}

func setDefaultListenerSocketOpts(fd int) error {
	// Allow reuse of recently-used addresses.
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1))
}
