//go:build windows

package sockets

import (
	"golang.org/x/sys/windows"
	"net"
	"syscall"
	"unsafe"
)

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

func sockaddrInet4ToRaw(rsa *windows.RawSockaddrAny, sa *windows.SockaddrInet4) int32 {
	*rsa = windows.RawSockaddrAny{}
	raw := (*windows.RawSockaddrInet4)(unsafe.Pointer(rsa))
	raw.Family = windows.AF_INET
	p := (*[2]byte)(unsafe.Pointer(&raw.Port))
	p[0] = byte(sa.Port >> 8)
	p[1] = byte(sa.Port)
	raw.Addr = sa.Addr
	return int32(unsafe.Sizeof(*raw))
}

func sockaddrInet6ToRaw(rsa *windows.RawSockaddrAny, sa *windows.SockaddrInet6) int32 {
	*rsa = windows.RawSockaddrAny{}
	raw := (*windows.RawSockaddrInet6)(unsafe.Pointer(rsa))
	raw.Family = windows.AF_INET6
	p := (*[2]byte)(unsafe.Pointer(&raw.Port))
	p[0] = byte(sa.Port >> 8)
	p[1] = byte(sa.Port)
	raw.Scope_id = sa.ZoneId
	raw.Addr = sa.Addr
	return int32(unsafe.Sizeof(*raw))
}

func sockaddrUnixToRaw(rsa *windows.RawSockaddrAny, sa *windows.SockaddrUnix) int32 {
	*rsa = windows.RawSockaddrAny{}
	raw := (*windows.RawSockaddrUnix)(unsafe.Pointer(rsa))
	raw.Family = windows.AF_UNIX
	path := make([]byte, 0, len(sa.Name))
	copy(path, sa.Name)
	n := 0
	for n < len(path) && path[n] != 0 {
		n++
	}
	pp := []int8(unsafe.Slice((*int8)(unsafe.Pointer(&path[0])), n))
	copy(raw.Path[:], pp)
	return int32(unsafe.Sizeof(*raw))
}

func sockaddrToRaw(rsa *windows.RawSockaddrAny, sa windows.Sockaddr) (int32, error) {
	switch sa := sa.(type) {
	case *windows.SockaddrInet4:
		sz := sockaddrInet4ToRaw(rsa, sa)
		return sz, nil
	case *windows.SockaddrInet6:
		sz := sockaddrInet6ToRaw(rsa, sa)
		return sz, nil
	case *windows.SockaddrUnix:
		sz := sockaddrUnixToRaw(rsa, sa)
		return sz, nil
	default:
		return 0, syscall.EWINDOWS
	}
}
