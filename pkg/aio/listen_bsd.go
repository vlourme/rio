//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

import (
	"net"
	"runtime"
	"syscall"
)

var (
	somaxconn = maxListenerBacklog()
)

func maxListenerBacklog() int {
	var (
		n   uint32
		err error
	)
	switch runtime.GOOS {
	case "darwin", "ios":
		n, err = syscall.SysctlUint32("kern.ipc.somaxconn")
	case "freebsd":
		n, err = syscall.SysctlUint32("kern.ipc.soacceptqueue")
	case "netbsd":
	case "openbsd":
		n, err = syscall.SysctlUint32("kern.somaxconn")
	}
	if n == 0 || err != nil {
		return syscall.SOMAXCONN
	}
	if n > 1<<16-1 {
		n = 1<<16 - 1
	}
	return int(n)
}

func setDeferAccept(_ int) error {
	return nil
}

func newListener(sock int, network string, family int, sotype int, proto int, ipv6only bool, addr net.Addr) *netFd {
	return newNetFd(sock, network, family, sotype, proto, ipv6only, addr, nil)
}
