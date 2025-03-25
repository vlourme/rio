package sys

import (
	"errors"
	"net"
	"reflect"
	"strings"
)

func ParseAddr(network string, address string) (addr net.Addr, err error) {
	address = strings.TrimSpace(address)
	if address == "" {
		err = errors.New("address is invalid")
		return
	}
	ipv6only := strings.HasSuffix(network, "6")
	proto := network
	if colon := strings.IndexByte(network, ':'); colon > -1 {
		proto = network[:colon]
	}
	switch proto {
	case "tcp", "tcp4", "tcp6":
		a, resolveErr := net.ResolveTCPAddr(network, address)
		if resolveErr != nil {
			err = resolveErr
			return
		}
		if len(a.IP) == 0 {
			a.IP = net.IPv6zero
		}
		if !ipv6only {
			if a.IP.Equal(net.IPv4zero) {
				a.IP = net.IPv6zero
			}
		}
		if !ipv6only && a.AddrPort().Addr().Is4In6() {
			a.IP = a.IP.To4()
		}
		addr = a
		break
	case "udp", "udp4", "udp6":
		a, resolveErr := net.ResolveUDPAddr(network, address)
		if resolveErr != nil {
			err = resolveErr
			return
		}
		if len(a.IP) == 0 {
			a.IP = net.IPv6zero
		}
		if !ipv6only {
			if a.IP.Equal(net.IPv4zero) {
				a.IP = net.IPv6zero
			}
		}
		if !ipv6only && a.AddrPort().Addr().Is4In6() {
			a.IP = a.IP.To4()
		}
		addr = a
		break
	case "ip", "ip4", "ip6":
		a, resolveErr := net.ResolveIPAddr(network, address)
		if resolveErr != nil {
			err = resolveErr
			return
		}
		ipLen := len(a.IP)
		if ipLen == 0 {
			a.IP = net.IPv6zero
		}
		if !ipv6only {
			if a.IP.Equal(net.IPv4zero) {
				a.IP = net.IPv6zero
			}
		}
		if !ipv6only && ipLen == net.IPv6len {
			if isZeros(a.IP[0:10]) && a.IP[10] == 0xff && a.IP[11] == 0xff {
				a.IP = a.IP.To4()
			}
		}
		addr = a
		break
	case "unix", "unixgram", "unixpacket":
		addr, err = net.ResolveUnixAddr(network, address)
		if err != nil {
			return
		}
		break
	default:
		err = errors.New("network is invalid")
		return
	}
	return
}

func ResolveAddresses(network string, address string) (addrs []net.Addr, err error) {
	network = strings.TrimSpace(network)
	if network == "" {
		err = errors.New("missing network")
		return
	}
	address = strings.TrimSpace(address)
	if address == "" {
		err = errors.New("missing address")
		return
	}
	switch network {
	case "unix", "unixgram", "unixpacket":
		addr, addrErr := ParseAddr(network, address)
		if addrErr != nil {
			err = addrErr
			return
		}
		addrs = append(addrs, addr)
		return
	default:
		break
	}
	var (
		host string
		port string
	)
	host, port, err = net.SplitHostPort(address)
	if err != nil {
		return
	}
	if port == "" {
		switch network {
		case "ip", "ip4", "ip6":
			err = errors.New("missing port")
			return
		default:
			break
		}
	}
	if host == "" {
		addr, addrErr := ParseAddr(network, address)
		if addrErr != nil {
			err = addrErr
			return
		}
		addrs = append(addrs, addr)
		return
	}

	ips, ipsErr := net.LookupHost(host)
	if ipsErr != nil {
		err = ipsErr
		return
	}
	if len(ips) == 0 {
		err = errors.New("ip is not found by host")
		return
	}
	ipv6only := strings.HasSuffix(network, "6")
	ipv4only := strings.HasSuffix(network, "4")
	for _, ip := range ips {
		ipv6 := strings.Contains(ip, ":")
		if ipv6 && ipv4only {
			continue
		}
		if !ipv6 && ipv6only {
			continue
		}
		address = net.JoinHostPort(ip, port)
		addr, addrErr := ParseAddr(network, address)
		if addrErr != nil {
			err = addrErr
			return
		}
		addrs = append(addrs, addr)
	}
	return
}

// IsIPv4 reports whether addr contains an IPv4 address.
func IsIPv4(addr net.Addr) bool {
	switch addr := addr.(type) {
	case *net.TCPAddr:
		return addr.IP.To4() != nil
	case *net.UDPAddr:
		return addr.IP.To4() != nil
	case *net.IPAddr:
		return addr.IP.To4() != nil
	}
	return false
}

// IsNotIPv4 reports whether addr does not contain an IPv4 address.
func IsNotIPv4(addr net.Addr) bool { return !IsIPv4(addr) }

func FilterAddresses(addrs []net.Addr, hint net.Addr) (v []net.Addr) {
	if hint == nil {
		return addrs
	}
	hintV4 := IsIPv4(hint)
	for _, addr := range addrs {
		if reflect.ValueOf(addr).IsNil() {
			continue
		}
		if addr.Network() != hint.Network() {
			continue
		}
		v4 := IsIPv4(addr)
		if !v4 && hintV4 {
			continue
		}
		if v4 && !hintV4 {
			continue
		}
		v = append(v, addr)
	}
	return
}

// PartitionAddrs divides an address list into two categories, using a
// strategy function to assign a boolean label to each address.
// The first address, and any with a matching label, are returned as
// primaries, while addresses with the opposite label are returned
// as fallbacks. For non-empty inputs, primaries is guaranteed to be
// non-empty.
func PartitionAddrs(addrs []net.Addr, strategy func(net.Addr) bool) (primaries, fallbacks []net.Addr) {
	var primaryLabel bool
	for i, addr := range addrs {
		label := strategy(addr)
		if i == 0 || label == primaryLabel {
			primaryLabel = label
			primaries = append(primaries, addr)
		} else {
			fallbacks = append(fallbacks, addr)
		}
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
