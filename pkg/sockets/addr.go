package sockets

import (
	"net"
	"syscall"
	_ "unsafe"
)

func getAddrFamily(network string, addr net.Addr) (family int, ipv6only bool) {
	switch network {
	case "tcp", "udp":
		switch add := addr.(type) {
		case *net.TCPAddr:
			ipv6only = add.AddrPort().Addr().Is6()
			if ipv6only {
				family = syscall.AF_INET6
			}
			break
		case *net.UDPAddr:
			ipv6only = add.AddrPort().Addr().Is6()
			if ipv6only {
				family = syscall.AF_INET6
			}
			break
		default:
			family = syscall.AF_UNSPEC
			break
		}
	case "tcp4", "udp4":
		family = syscall.AF_INET
	case "tcp6", "udp6":
		family = syscall.AF_INET6
		ipv6only = true
		break
	case "unix", "unixgram", "unixpacket":
		family = syscall.AF_UNIX
		break
	default:
		family = syscall.AF_UNSPEC
		break
	}
	return
}
