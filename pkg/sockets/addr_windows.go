//go:build windows

package sockets

import (
	"golang.org/x/sys/windows"
	"net"
)

func sockaddrToTCPAddr(sa windows.Sockaddr) (addr *net.TCPAddr) {
	switch sa := sa.(type) {
	case *windows.SockaddrInet4:
		addr = &net.TCPAddr{
			IP:   append([]byte{}, sa.Addr[:]...),
			Port: sa.Port,
		}
	case *windows.SockaddrInet6:
		var zone string
		if sa.ZoneId != 0 {
			if ifi, err := net.InterfaceByIndex(int(sa.ZoneId)); err == nil {
				zone = ifi.Name
			}
		}
		if zone == "" && sa.ZoneId != 0 {
		}
		addr = &net.TCPAddr{
			IP:   append([]byte{}, sa.Addr[:]...),
			Port: sa.Port,
			Zone: zone,
		}
	}
	return
}

func sockaddrToUnixAddr(sa windows.Sockaddr) net.Addr {
	var a net.Addr
	switch sa := sa.(type) {
	case *windows.SockaddrUnix:
		a = &net.UnixAddr{Net: "unix", Name: sa.Name}
	}
	return a
}

func sockaddrToUDPAddr(sa windows.Sockaddr) (addr *net.UDPAddr) {
	switch sa := sa.(type) {
	case *windows.SockaddrInet4:
		addr = &net.UDPAddr{
			IP:   append([]byte{}, sa.Addr[:]...),
			Port: sa.Port,
		}
	case *windows.SockaddrInet6:
		var zone string
		if sa.ZoneId != 0 {
			if ifi, err := net.InterfaceByIndex(int(sa.ZoneId)); err == nil {
				zone = ifi.Name
			}
		}
		if zone == "" && sa.ZoneId != 0 {
		}
		addr = &net.UDPAddr{
			IP:   append([]byte{}, sa.Addr[:]...),
			Port: sa.Port,
			Zone: zone,
		}
	}
	return
}

func sockaddrToAddr(network string, sa windows.Sockaddr) (addr net.Addr) {
	switch sa := sa.(type) {
	case *windows.SockaddrInet4:
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
	case *windows.SockaddrInet6:
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
	case *windows.SockaddrUnix:
		addr = &net.UnixAddr{Net: network, Name: sa.Name}
		break
	}
	return
}

func addrToSockaddr(family int, a net.Addr) (sa windows.Sockaddr) {
	switch addr := a.(type) {
	case *net.TCPAddr:
		switch family {
		case windows.AF_INET:
			sa4 := &windows.SockaddrInet4{
				Port: addr.Port,
				Addr: [4]byte{},
			}
			ip := addr.IP
			if len(ip) == 0 {
				ip = net.IPv4zero
			}
			copy(sa4.Addr[:], ip.To4())
			sa = sa4
			break
		case windows.AF_INET6:
			sa4 := &windows.SockaddrInet6{
				Port: addr.Port,
				Addr: [16]byte{},
			}
			ip := addr.IP
			if len(ip) == 0 {
				ip = net.IPv6zero
			}
			copy(sa4.Addr[:], ip.To16())
			sa = sa4
			break
		}
		break
	case *net.UDPAddr:
		switch family {
		case windows.AF_INET:
			sa4 := &windows.SockaddrInet4{
				Port: addr.Port,
				Addr: [4]byte{},
			}
			copy(sa4.Addr[:], addr.IP.To4())
			sa = sa4
			break
		case windows.AF_INET6:
			sa4 := &windows.SockaddrInet6{
				Port: addr.Port,
				Addr: [16]byte{},
			}
			copy(sa4.Addr[:], addr.IP.To16())
			sa = sa4
			break
		}
		break
	case *net.UnixAddr:
		sa = &windows.SockaddrUnix{
			Name: addr.Name,
		}
		break
	}
	return
}
