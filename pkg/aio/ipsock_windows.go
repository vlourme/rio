//go:build windows

package aio

import (
	"net"
	"syscall"
)

func (p *ipStackCapabilities) probe() {
	s, err := sysSocket(syscall.AF_INET, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
	switch err {
	case syscall.EAFNOSUPPORT, syscall.EPROTONOSUPPORT:
	case nil:
		_ = syscall.Close(syscall.Handle(s))
		p.ipv4Enabled = true
	}
	var probes = []struct {
		laddr net.TCPAddr
		value int
	}{
		{laddr: net.TCPAddr{IP: net.ParseIP("::1")}, value: 1},
		{laddr: net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)}, value: 0},
	}
	for i := range probes {
		s, err = sysSocket(syscall.AF_INET6, syscall.SOCK_STREAM, syscall.IPPROTO_TCP)
		if err != nil {
			continue
		}
		defer syscall.Close(syscall.Handle(s))
		syscall.SetsockoptInt(syscall.Handle(s), syscall.IPPROTO_IPV6, syscall.IPV6_V6ONLY, probes[i].value)
		sa, err := tcpAddrToSockaddr(syscall.AF_INET6, &probes[i].laddr)
		if err != nil {
			continue
		}
		if err := syscall.Bind(syscall.Handle(s), sa); err != nil {
			continue
		}
		if i == 0 {
			p.ipv6Enabled = true
		} else {
			p.ipv4MappedIPv6Enabled = true
		}
	}
}
