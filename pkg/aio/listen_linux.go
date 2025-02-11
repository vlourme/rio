//go:build linux

package aio

import (
	"bufio"
	"net"
	"os"
	"syscall"
)

var (
	somaxconn = maxListenerBacklog()
)

func maxListenerBacklog() int {
	fd, err := os.Open("/proc/sys/net/core/somaxconn")
	if err != nil {
		return syscall.SOMAXCONN
	}
	defer fd.Close()
	rd := bufio.NewReader(fd)

	l, readLineErr := rd.ReadString('\n')
	if readLineErr != nil {
		return syscall.SOMAXCONN
	}
	f := getFields(l)
	n, _, ok := dtoi(f[0])
	if n == 0 || !ok {
		return syscall.SOMAXCONN
	}

	if n > 1<<16-1 {
		return maxAckBacklog(n)
	}
	return n
}

func maxAckBacklog(n int) int {
	major, minor := KernelVersion()
	size := 16
	if major > 4 || (major == 4 && minor >= 1) {
		size = 32
	}

	var maxAck uint = 1<<size - 1
	if uint(n) > maxAck {
		n = int(maxAck)
	}
	return n
}

func setDeferAccept(sock int) error {
	return os.NewSyscallError("setsockopt", syscall.SetsockoptInt(sock, syscall.IPPROTO_TCP, syscall.TCP_DEFER_ACCEPT, 1))
}

func newListener(sock int, network string, family int, sotype int, proto int, ipv6only bool, addr net.Addr) *netFd {
	cylinder := nextIOURingCylinder()
	return newNetFd(cylinder, sock, network, family, sotype, proto, ipv6only, addr, nil)
}
