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
		err = errors.New("sockets: invalid addr")
		return
	}
	// unix
	if network == "unix" || network == "unixgram" || network == "unixpacket" {
		v, err = net.ResolveUnixAddr(network, addr)
		if err == nil {
			return
		}
		family = syscall.AF_UNIX
		return
	}
	// parse addr
	ap, parseAddrErr := ParseAddrPort(addr)
	if parseAddrErr != nil {
		err = parseAddrErr
		return
	}
	if !ap.IsValid() {
		err = errors.New("sockets: invalid addr")
		return
	}
	ip := ap.Addr().AsSlice()
	ipLen := len(ip)
	if ipLen == net.IPv6len {
		ipv6only = true
	}
	port := int(ap.Port())
	switch network {
	case "tcp":
		if ipv6only {
			v = &net.TCPAddr{
				IP:   ip,
				Port: port,
				Zone: ap.Addr().Zone(),
			}
			family = syscall.AF_INET6
		} else {
			if ipLen == 0 {
				ip = net.IPv4zero
			}
			v = &net.TCPAddr{
				IP:   ip,
				Port: port,
				Zone: "",
			}
			family = syscall.AF_INET
		}
		break
	case "tcp4":
		if ipv6only {
			err = errors.New("sockets: tcp4 is not supported on IPv4")
			return
		}
		if ipLen == 0 {
			ip = net.IPv4zero
		}
		v = &net.TCPAddr{
			IP:   ip,
			Port: port,
			Zone: "",
		}
		family = syscall.AF_INET
		break
	case "tcp6":
		if ipv6only {
			err = errors.New("sockets: tcp6 is not supported on IPv6")
			return
		}
		v = &net.TCPAddr{
			IP:   ip,
			Port: port,
			Zone: ap.Addr().Zone(),
		}
		family = syscall.AF_INET6
		break
	case "udp":
		if ipv6only {
			v = &net.UDPAddr{
				IP:   ip,
				Port: port,
				Zone: ap.Addr().Zone(),
			}
			family = syscall.AF_INET6
		} else {
			if ipLen == 0 {
				ip = net.IPv4zero
			}
			v = &net.UDPAddr{
				IP:   ip,
				Port: port,
				Zone: "",
			}
			family = syscall.AF_INET
		}
		break
	case "udp4":
		if ipv6only {
			err = errors.New("sockets: udp4 is not supported on IPv4")
			return
		}
		if ipLen == 0 {
			ip = net.IPv4zero
		}
		v = &net.UDPAddr{
			IP:   ip,
			Port: port,
			Zone: "",
		}
		family = syscall.AF_INET
		break
	case "udp6":
		if ipv6only {
			err = errors.New("sockets: udp6 is not supported on IPv6")
			return
		}
		v = &net.UDPAddr{
			IP:   ip,
			Port: port,
			Zone: ap.Addr().Zone(),
		}
		family = syscall.AF_INET6
		break
	}
	return
}

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
