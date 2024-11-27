package sockets

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"syscall"
)

func GetAddrAndFamily(network string, addr string) (v net.Addr, family int, ipv6only bool, err error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		err = errors.New("aio.ResolveAddr: invalid addr")
		return
	}
	ipv6only = strings.HasSuffix(network, "6")
	switch network {
	case "tcp", "tcp4", "tcp6":
		a, resolveErr := net.ResolveTCPAddr(network, addr)
		if resolveErr != nil {
			err = errors.New("aio.ResolveAddr: " + resolveErr.Error())
			return
		}
		if !ipv6only && a.AddrPort().Addr().Is4In6() {
			a.IP = a.IP.To4()
		}
		switch len(a.IP) {
		case net.IPv4len:
			family = syscall.AF_INET
			break
		case net.IPv6len:
			family = syscall.AF_INET6
			break
		case 0:
			family = syscall.AF_INET
			a.IP = net.IPv4zero.To4()
			break
		default:
			err = errors.New("aio.ResolveAddr: invalid ip length")
			return
		}
		v = a
		break
	case "udp", "udp4", "udp6":
		a, resolveErr := net.ResolveUDPAddr(network, addr)
		if resolveErr != nil {
			err = errors.New("aio.ResolveAddr: " + resolveErr.Error())
			return
		}
		if !ipv6only && a.AddrPort().Addr().Is4In6() {
			a.IP = a.IP.To4()
		}
		switch len(a.IP) {
		case net.IPv4len:
			family = syscall.AF_INET
			break
		case net.IPv6len:
			family = syscall.AF_INET6
			break
		case 0:
			family = syscall.AF_INET
			a.IP = net.IPv4zero.To4()
			break
		default:
			err = errors.New("aio.ResolveAddr: invalid ip length")
			return
		}
		v = a
		break
	case "ip", "ip4", "ip6":
		a, resolveErr := net.ResolveIPAddr(network, addr)
		if resolveErr != nil {
			err = errors.New("aio.ResolveAddr: " + resolveErr.Error())
			return
		}
		ipLen := len(a.IP)
		if !ipv6only && ipLen == net.IPv6len {
			if isZeros(a.IP[0:10]) && a.IP[10] == 0xff && a.IP[11] == 0xff {
				a.IP = a.IP.To4()
			}
		}
		switch ipLen {
		case net.IPv4len:
			family = syscall.AF_INET
			break
		case net.IPv6len:
			family = syscall.AF_INET6
			break
		case 0:
			family = syscall.AF_INET
			a.IP = net.IPv4zero.To4()
			break
		default:
			err = errors.New("aio.ResolveAddr: invalid ip length")
			return
		}
		v = a
		break
	case "unix", "unixgram", "unixpacket":
		family = syscall.AF_UNIX
		v, err = net.ResolveUnixAddr(network, addr)
		if err != nil {
			err = errors.New("aio.ResolveAddr: " + err.Error())
			return
		}
		break
	default:
		err = errors.New("aio.ResolveAddr: invalid network")
		return
	}
	return
}

func isZeros(p net.IP) bool {
	for i := 0; i < len(p); i++ {
		if p[i] != 0 {
			return false
		}
	}
	return true
}

//
//func GetAddrAndFamily(network string, addr string) (v net.Addr, family int, ipv6only bool, err error) {
//	addr = strings.TrimSpace(addr)
//	if addr == "" {
//		err = errors.New("sockets: invalid addr")
//		return
//	}
//	switch network {
//	case "tcp", "tcp4", "tcp6":
//		a, resolveErr := net.ResolveTCPAddr(network, addr)
//		if resolveErr != nil {
//			err = resolveErr
//			return
//		}
//		if a.AddrPort().Addr().Is6() {
//			family = syscall.AF_INET6
//			ipv6only = true
//		} else {
//			family = syscall.AF_INET
//		}
//		break
//	case "udp", "udp4", "udp6":
//		v, err = net.ResolveUDPAddr(network, addr)
//		break
//	case "ip", "ip4", "ip6":
//		v, err = net.ResolveIPAddr(network, addr)
//		break
//	case "unix", "unixgram", "unixpacket":
//		family = syscall.AF_UNIX
//		v, err = net.ResolveUnixAddr(network, addr)
//		break
//	default:
//		err = errors.New("sockets: invalid network")
//		return
//	}
//
//	// ip
//	if network == "ip" || network == "ipv4" || network == "ipv6" {
//		v, err = net.ResolveIPAddr(network, addr)
//		if err != nil {
//			return
//		}
//		if v.Network() == "ipv6" {
//			family = syscall.AF_INET6
//		} else {
//			family = syscall.AF_INET
//		}
//		family = syscall.AF_INET
//		return
//	}
//	// unix
//	if network == "unix" || network == "unixgram" || network == "unixpacket" {
//		v, err = net.ResolveUnixAddr(network, addr)
//		if err != nil {
//			return
//		}
//		family = syscall.AF_UNIX
//		return
//	}
//	// parse addr
//	ap, parseAddrErr := ParseAddrPort(addr)
//	if parseAddrErr != nil {
//		err = parseAddrErr
//		return
//	}
//	if !ap.IsValid() {
//		err = errors.New("sockets: invalid addr")
//		return
//	}
//	ip := ap.Addr().AsSlice()
//	ipLen := len(ip)
//	if ipLen == net.IPv6len {
//		ipv6only = true
//	}
//	port := int(ap.Port())
//	switch network {
//	case "tcp":
//		if ipv6only {
//			v = &net.TCPAddr{
//				IP:   ip,
//				Port: port,
//				Zone: ap.Addr().Zone(),
//			}
//			family = syscall.AF_INET6
//		} else {
//			if ipLen == 0 {
//				ip = net.IPv4zero
//			}
//			v = &net.TCPAddr{
//				IP:   ip,
//				Port: port,
//				Zone: "",
//			}
//			family = syscall.AF_INET
//		}
//		break
//	case "tcp4":
//		if ipv6only {
//			err = errors.New("sockets: tcp4 is not supported on IPv4")
//			return
//		}
//		if ipLen == 0 {
//			ip = net.IPv4zero
//		}
//		v = &net.TCPAddr{
//			IP:   ip,
//			Port: port,
//			Zone: "",
//		}
//		family = syscall.AF_INET
//		break
//	case "tcp6":
//		if ipv6only {
//			err = errors.New("sockets: tcp6 is not supported on IPv6")
//			return
//		}
//		v = &net.TCPAddr{
//			IP:   ip,
//			Port: port,
//			Zone: ap.Addr().Zone(),
//		}
//		family = syscall.AF_INET6
//		break
//	case "udp":
//		if ipv6only {
//			v = &net.UDPAddr{
//				IP:   ip,
//				Port: port,
//				Zone: ap.Addr().Zone(),
//			}
//			family = syscall.AF_INET6
//		} else {
//			if ipLen == 0 {
//				ip = net.IPv4zero
//			}
//			v = &net.UDPAddr{
//				IP:   ip,
//				Port: port,
//				Zone: "",
//			}
//			family = syscall.AF_INET
//		}
//		break
//	case "udp4":
//		if ipv6only {
//			err = errors.New("sockets: udp4 is not supported on IPv4")
//			return
//		}
//		if ipLen == 0 {
//			ip = net.IPv4zero
//		}
//		v = &net.UDPAddr{
//			IP:   ip,
//			Port: port,
//			Zone: "",
//		}
//		family = syscall.AF_INET
//		break
//	case "udp6":
//		if ipv6only {
//			err = errors.New("sockets: udp6 is not supported on IPv6")
//			return
//		}
//		v = &net.UDPAddr{
//			IP:   ip,
//			Port: port,
//			Zone: ap.Addr().Zone(),
//		}
//		family = syscall.AF_INET6
//		break
//	}
//	return
//}

func ParseAddrPort(addr string) (addrPort netip.AddrPort, err error) {
	i := strings.LastIndexByte(addr, ':')
	if i == -1 {
		err = errors.New("sockets: not an ip:port")
		return
	}
	ip, port := addr[:i], addr[i+1:]
	if len(port) == 0 {
		err = errors.New("sockets: no port")
		return
	}
	if len(ip) == 0 {
		ip = "0.0.0.0"
	} else {
		if ip[0] == '[' {
			if len(ip) < 2 || ip[len(ip)-1] != ']' {
				err = errors.New("sockets: invalid ipv6")
				return
			}
		}
	}
	portNum, portErr := strconv.Atoi(port)
	if portErr != nil {
		err = errors.New("sockets: invalid port")
		return
	}
	addr = fmt.Sprintf("%s:%d", ip, portNum)
	addrPort, err = netip.ParseAddrPort(addr)
	return
}

func SockaddrToAddr(network string, sa syscall.Sockaddr) (addr net.Addr) {
	switch sa := sa.(type) {
	case *syscall.SockaddrInet4:
		switch network {
		case "tcp", "tcp4":
			addr = &net.TCPAddr{
				IP:   append([]byte{}, sa.Addr[:]...),
				Port: sa.Port,
			}
			break
		case "udp", "udp4":
			addr = &net.UDPAddr{
				IP:   append([]byte{}, sa.Addr[:]...),
				Port: sa.Port,
			}
			break
		}
	case *syscall.SockaddrInet6:
		var zone string
		if sa.ZoneId != 0 {
			if ifi, err := net.InterfaceByIndex(int(sa.ZoneId)); err == nil {
				zone = ifi.Name
			}
		}
		if zone == "" && sa.ZoneId != 0 {
		}
		switch network {
		case "tcp", "tcp6":
			addr = &net.TCPAddr{
				IP:   append([]byte{}, sa.Addr[:]...),
				Port: sa.Port,
				Zone: zone,
			}
			break
		case "udp", "udp6":
			addr = &net.UDPAddr{
				IP:   append([]byte{}, sa.Addr[:]...),
				Port: sa.Port,
				Zone: zone,
			}
			break
		}
	case *syscall.SockaddrUnix:
		addr = &net.UnixAddr{Net: network, Name: sa.Name}
		break
	}
	return
}
