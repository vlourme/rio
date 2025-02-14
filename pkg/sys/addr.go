package sys

import (
	"github.com/brickingsoft/errors"
	"net"
	"strings"
	"syscall"
	"unsafe"
)

func ResolveAddr(network string, address string) (addr net.Addr, family int, ipv6only bool, err error) {
	address = strings.TrimSpace(address)
	if address == "" {
		err = errors.New("resolve address failed", errors.WithWrap(errors.New("address is invalid")))
		return
	}
	proto := network
	if colon := strings.IndexByte(network, ':'); colon > -1 {
		proto = network[:colon]
	}
	ipv6only = strings.HasSuffix(network, "6")
	switch proto {
	case "tcp", "tcp4", "tcp6":
		a, resolveErr := net.ResolveTCPAddr(network, address)
		if resolveErr != nil {
			err = errors.New("resolve address failed", errors.WithWrap(resolveErr))
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
			err = errors.New("resolve address failed", errors.WithWrap(errors.New("ip is invalid")))
			return
		}
		addr = a
		break
	case "udp", "udp4", "udp6":
		a, resolveErr := net.ResolveUDPAddr(network, address)
		if resolveErr != nil {
			err = errors.New("resolve address failed", errors.WithWrap(resolveErr))
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
			err = errors.New("resolve address failed", errors.WithWrap(errors.New("ip is invalid")))
			return
		}
		addr = a
		break
	case "ip", "ip4", "ip6":
		a, resolveErr := net.ResolveIPAddr(network, address)
		if resolveErr != nil {
			err = errors.New("resolve address failed", errors.WithWrap(resolveErr))
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
			err = errors.New("resolve address failed", errors.WithWrap(errors.New("ip is invalid")))
			return
		}
		addr = a
		break
	case "unix", "unixgram", "unixpacket":
		family = syscall.AF_UNIX
		addr, err = net.ResolveUnixAddr(network, address)
		if err != nil {
			err = errors.New("resolve address failed", errors.WithWrap(err))
			return
		}
		break
	default:
		err = errors.New("resolve address failed", errors.WithWrap(errors.New("network is invalid")))
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

func AddrToSockaddr(a net.Addr) (sa syscall.Sockaddr, err error) {
	switch addr := a.(type) {
	case *net.TCPAddr:
		if addr.AddrPort().Addr().Is4In6() {
			addr.IP = addr.IP.To4()
		}
		switch len(addr.IP) {
		case net.IPv4len:
			sa4 := &syscall.SockaddrInet4{
				Port: addr.Port,
				Addr: [4]byte{},
			}
			copy(sa4.Addr[:], addr.IP.To4())
			sa = sa4
			return
		case net.IPv6len:
			zoneId := uint32(0)
			if ifi, ifiErr := net.InterfaceByName(addr.Zone); ifiErr == nil {
				zoneId = uint32(ifi.Index)
			}
			sa6 := &syscall.SockaddrInet6{
				Port:   addr.Port,
				Addr:   [16]byte{},
				ZoneId: zoneId,
			}
			copy(sa6.Addr[:], addr.IP.To16())
			sa = sa6
			return
		default:
			err = errors.New("map addr to socket addr failed", errors.WithWrap(errors.New("ip is invalid")))
			return
		}
	case *net.UDPAddr:
		if addr.AddrPort().Addr().Is4In6() {
			addr.IP = addr.IP.To4()
		}
		switch len(addr.IP) {
		case net.IPv4len:
			sa4 := &syscall.SockaddrInet4{
				Port: addr.Port,
				Addr: [4]byte{},
			}
			copy(sa4.Addr[:], addr.IP)
			sa = sa4
			return
		case net.IPv6len:
			zoneId := uint32(0)
			if ifi, ifiErr := net.InterfaceByName(addr.Zone); ifiErr == nil {
				zoneId = uint32(ifi.Index)
			}
			sa6 := &syscall.SockaddrInet6{
				Port:   addr.Port,
				Addr:   [16]byte{},
				ZoneId: zoneId,
			}
			copy(sa6.Addr[:], addr.IP)
			sa = sa6
			return
		default:
			err = errors.New("map addr to socket addr failed", errors.WithWrap(errors.New("ip is invalid")))
			return
		}
	case *net.IPAddr:
		ipLen := len(addr.IP)
		if ipLen == net.IPv6len && isZeros(addr.IP[0:10]) && addr.IP[10] == 0xff && addr.IP[11] == 0xff {
			addr.IP = addr.IP.To4()
		}
		switch ipLen {
		case net.IPv4len:
			sa4 := &syscall.SockaddrInet4{
				Port: 0,
				Addr: [4]byte{},
			}
			copy(sa4.Addr[:], addr.IP)
			sa = sa4
			return
		case net.IPv6len:
			zoneId := uint32(0)
			if ifi, ifiErr := net.InterfaceByName(addr.Zone); ifiErr == nil {
				zoneId = uint32(ifi.Index)
			}
			sa6 := &syscall.SockaddrInet6{
				Port:   0,
				Addr:   [16]byte{},
				ZoneId: zoneId,
			}
			copy(sa6.Addr[:], addr.IP)
			sa = sa6
			return
		default:
			err = errors.New("map addr to socket addr failed", errors.WithWrap(errors.New("ip is invalid")))
			return
		}
	case *net.UnixAddr:
		sa = &syscall.SockaddrUnix{
			Name: addr.Name,
		}
		return
	default:
		err = errors.New("map addr to socket addr failed", errors.WithWrap(errors.New("ip is invalid")))
		return
	}
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
		case "ip", "ip4":
			addr = &net.IPAddr{
				IP:   append([]byte{}, sa.Addr[:]...),
				Zone: "",
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
		case "ip", "ip6":
			addr = &net.IPAddr{
				IP:   append([]byte{}, sa.Addr[:]...),
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

func RawSockaddrAnyToSockaddr(rsa *syscall.RawSockaddrAny) (syscall.Sockaddr, error) {
	switch rsa.Addr.Family {
	case syscall.AF_NETLINK:
		pp := (*syscall.RawSockaddrNetlink)(unsafe.Pointer(rsa))
		sa := new(syscall.SockaddrNetlink)
		sa.Family = pp.Family
		sa.Pad = pp.Pad
		sa.Pid = pp.Pid
		sa.Groups = pp.Groups
		return sa, nil

	case syscall.AF_PACKET:
		pp := (*syscall.RawSockaddrLinklayer)(unsafe.Pointer(rsa))
		sa := new(syscall.SockaddrLinklayer)
		sa.Protocol = pp.Protocol
		sa.Ifindex = int(pp.Ifindex)
		sa.Hatype = pp.Hatype
		sa.Pkttype = pp.Pkttype
		sa.Halen = pp.Halen
		sa.Addr = pp.Addr
		return sa, nil

	case syscall.AF_UNIX:
		pp := (*syscall.RawSockaddrUnix)(unsafe.Pointer(rsa))
		sa := new(syscall.SockaddrUnix)
		if pp.Path[0] == 0 {
			// "Abstract" Unix domain socket.
			// Rewrite leading NUL as @ for textual display.
			// (This is the standard convention.)
			// Not friendly to overwrite in place,
			// but the callers below don't care.
			pp.Path[0] = '@'
		}

		// Assume path ends at NUL.
		// This is not technically the Linux semantics for
		// abstract Unix domain sockets--they are supposed
		// to be uninterpreted fixed-size binary blobs--but
		// everyone uses this convention.
		n := 0
		for n < len(pp.Path) && pp.Path[n] != 0 {
			n++
		}
		bytes := (*[len(pp.Path)]byte)(unsafe.Pointer(&pp.Path[0]))[0:n]
		sa.Name = string(bytes)
		return sa, nil

	case syscall.AF_INET:
		pp := (*syscall.RawSockaddrInet4)(unsafe.Pointer(rsa))
		sa := new(syscall.SockaddrInet4)
		p := (*[2]byte)(unsafe.Pointer(&pp.Port))
		sa.Port = int(p[0])<<8 + int(p[1])
		sa.Addr = pp.Addr
		return sa, nil

	case syscall.AF_INET6:
		pp := (*syscall.RawSockaddrInet6)(unsafe.Pointer(rsa))
		sa := new(syscall.SockaddrInet6)
		p := (*[2]byte)(unsafe.Pointer(&pp.Port))
		sa.Port = int(p[0])<<8 + int(p[1])
		sa.ZoneId = pp.Scope_id
		sa.Addr = pp.Addr
		return sa, nil
	}
	return nil, syscall.EAFNOSUPPORT
}
