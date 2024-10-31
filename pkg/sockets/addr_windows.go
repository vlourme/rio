//go:build windows

package sockets

import (
	"golang.org/x/sys/windows"
	"net"
)

func SockaddrToAddr(sa windows.Sockaddr) net.Addr {
	var a net.Addr
	switch sa := sa.(type) {
	case *windows.SockaddrInet4:
		a = &net.TCPAddr{
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
		a = &net.TCPAddr{
			IP:   append([]byte{}, sa.Addr[:]...),
			Port: sa.Port,
			Zone: zone,
		}
	case *windows.SockaddrUnix:
		a = &net.UnixAddr{Net: "unix", Name: sa.Name}
	}
	return a
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
