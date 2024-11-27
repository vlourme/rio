package aio

import (
	"errors"
	"net"
	"strings"
	"syscall"
	"unsafe"
)

func ResolveAddr(network string, addr string) (v net.Addr, family int, ipv6only bool, err error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		err = errors.New("aio.ResolveAddr: invalid addr")
		return
	}
	proto := network
	if colon := strings.IndexByte(network, ':'); colon > -1 {
		proto = network[:colon]
	}
	ipv6only = strings.HasSuffix(network, "6")
	switch proto {
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

func ParseIpProto(network string) (n string, proto int, err error) {
	i := strings.Index(network, ":")
	if i < 0 {
		n = network
		return
	}
	n = network[:i]
	protoName := network[i+1:]
	proto0, idx, ok := dtoi(protoName)
	if ok && idx == len(protoName) {
		proto = proto0
		return
	}
	p, getProtoErr := syscall.GetProtoByName(protoName)
	if getProtoErr != nil {
		err = errors.New("aio.ParseIpProto: " + getProtoErr.Error())
		return
	}
	proto = int(p.Proto)
	return
}

const big = 0xFFFFFF

func dtoi(s string) (n int, i int, ok bool) {
	n = 0
	for i = 0; i < len(s) && '0' <= s[i] && s[i] <= '9'; i++ {
		n = n*10 + int(s[i]-'0')
		if n >= big {
			return big, i, false
		}
	}
	if i == 0 {
		return 0, 0, false
	}
	return n, i, true
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

func AddrToSockaddr(a net.Addr) (sa syscall.Sockaddr) {
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
			panic(errors.New("aio.AddrToSockaddr: ip is invalid"))
			return nil
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
			panic(errors.New("aio.AddrToSockaddr: ip is invalid"))
			return nil
		}
		return
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
			panic(errors.New("aio.AddrToSockaddr: ip is invalid"))
			return nil
		}
		return
	case *net.UnixAddr:
		sa = &syscall.SockaddrUnix{
			Name: addr.Name,
		}
		return
	default:
		panic(errors.New("aio.AddrToSockaddr: addr type is invalid"))
		return nil
	}
	return
}

func RawToSockaddr(rsa *syscall.RawSockaddrAny) (syscall.Sockaddr, error) {
	switch rsa.Addr.Family {
	case syscall.AF_UNIX:
		pp := (*syscall.RawSockaddrUnix)(unsafe.Pointer(rsa))
		sa := new(syscall.SockaddrUnix)
		if pp.Path[0] == 0 {
			pp.Path[0] = '@'
		}
		n := 0
		for n < len(pp.Path) && pp.Path[n] != 0 {
			n++
		}
		sa.Name = string(unsafe.Slice((*byte)(unsafe.Pointer(&pp.Path[0])), n))
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
	return nil, errors.New("aio.AddrToSockaddr: sockaddr type is invalid")
}
