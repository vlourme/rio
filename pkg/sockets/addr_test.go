package sockets_test

import (
	"github.com/brickingsoft/rio/pkg/sockets"
	"syscall"
	"testing"
)

func TestGetAddrAndFamily(t *testing.T) {
	aps := []string{
		"127.0.0.1:8080",
		":8080",
		"[::]:888",
		"[fe80::e243:1f44:1650:563e%23]:8080",
	}
	for _, network := range []string{"tcp", "tcp4", "tcp6", "udp", "udp4", "udp6"} {
		for _, ap := range aps {
			addr, family, ipv6, err := sockets.GetAddrAndFamily(network, ap)
			fn := "unknown"
			switch family {
			case syscall.AF_INET:
				fn = "AF_INET"
				break
			case syscall.AF_INET6:
				fn = "AF_INET6"
				break
			}
			t.Log(network, ap, "->", fn, addr, ipv6, err)
		}
	}
}
