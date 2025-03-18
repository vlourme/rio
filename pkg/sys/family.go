//go:build linux

package sys

import (
	"net"
	"syscall"
)

func FavoriteAddrFamily(network string, laddr, raddr net.Addr, mode string) (family int, ipv6only bool) {
	switch network {
	case "unix", "unixgram", "unixpacket":
		family = syscall.AF_UNIX
		return
	default:
		break
	}
	switch network[len(network)-1] {
	case '4':
		return syscall.AF_INET, false
	case '6':
		return syscall.AF_INET6, true
	}

	if mode == "listen" && (laddr == nil || IsWildcard(laddr)) {
		if supportsIPv4map() || !supportsIPv4() {
			return syscall.AF_INET6, false
		}
		if laddr == nil {
			return syscall.AF_INET, false
		}
		return addrFamily(laddr), false
	}

	if (laddr == nil || addrFamily(laddr) == syscall.AF_INET) &&
		(raddr == nil || addrFamily(raddr) == syscall.AF_INET) {
		return syscall.AF_INET, false
	}
	return syscall.AF_INET6, false
}

func addrFamily(addr net.Addr) int {
	switch a := addr.(type) {
	case *net.TCPAddr:
		if a == nil || len(a.IP) <= net.IPv4len {
			return syscall.AF_INET
		}
		if a.IP.To4() != nil {
			return syscall.AF_INET
		}
		return syscall.AF_INET6
	case *net.UDPAddr:
		if a == nil || len(a.IP) <= net.IPv4len {
			return syscall.AF_INET
		}
		if a.IP.To4() != nil {
			return syscall.AF_INET
		}
		return syscall.AF_INET6
	case *net.UnixAddr:
		return syscall.AF_UNIX
	case *net.IPAddr:
		if a == nil || len(a.IP) <= net.IPv4len {
			return syscall.AF_INET
		}
		if a.IP.To4() != nil {
			return syscall.AF_INET
		}
		return syscall.AF_INET6
	default:
		return syscall.AF_UNSPEC
	}
}
