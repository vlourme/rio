package aio

import (
	"github.com/brickingsoft/errors"
	"net"
	"syscall"
)

type ConnectOptions struct {
	MultipathTCP bool
	LocalAddr    net.Addr
	FastOpen     int
}

func Connect(network string, address string, opts ConnectOptions, callback OperationCallback) {
	addr, family, ipv6only, addrErr := ResolveAddr(network, address)
	if addrErr != nil {
		err := errors.New(
			"connect failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpConnect),
			errors.WithWrap(addrErr),
		)
		callback(Userdata{}, err)
		return
	}
	switch network {
	case "tcp", "tcp4", "tcp6":
		tcpAddr := addr.(*net.TCPAddr)
		if tcpAddr.IP == nil || tcpAddr.IP.IsUnspecified() {
			if ipv6only {
				tcpAddr.IP = net.IPv6loopback
			} else {
				tcpAddr.IP = net.ParseIP("127.0.0.1").To4()
			}
		}
		proto := syscall.IPPROTO_TCP
		if opts.MultipathTCP {
			if multipathProto, ok := tryGetMultipathTCPProto(); ok {
				proto = multipathProto
			}
		}
		fastOpen := opts.FastOpen
		connect(network, family, syscall.SOCK_STREAM, proto, ipv6only, tcpAddr, nil, fastOpen, callback)
		break
	case "udp", "udp4", "udp6":
		udpAddr := addr.(*net.UDPAddr)
		if udpAddr.IP == nil || udpAddr.IP.IsUnspecified() {
			if ipv6only {
				udpAddr.IP = net.IPv6loopback
			} else {
				udpAddr.IP = net.ParseIP("127.0.0.1").To4()
			}
		}
		connect(network, family, syscall.SOCK_DGRAM, syscall.IPPROTO_UDP, ipv6only, udpAddr, opts.LocalAddr, 0, callback)
		break
	case "unix":
		connect(network, family, syscall.SOCK_STREAM, 0, ipv6only, addr, nil, 0, callback)
		break
	case "unixgram":
		connect(network, family, syscall.SOCK_DGRAM, 0, ipv6only, addr, nil, 0, callback)
		break
	case "unixpacket":
		connect(network, family, syscall.SOCK_SEQPACKET, 0, ipv6only, addr, nil, 0, callback)
		break
	case "ip", "ip4", "ip6":
		proto := 0
		var parseProtoError error
		network, proto, parseProtoError = ParseIpProto(network)
		if parseProtoError != nil {
			callback(Userdata{}, parseProtoError)
			return
		}
		connect(network, family, syscall.SOCK_RAW, proto, ipv6only, addr, nil, 0, callback)
		break
	default:
		err := errors.New(
			"connect failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpConnect),
			errors.WithWrap(errors.Define("network is not support")),
		)
		callback(Userdata{}, err)
		return
	}
	return
}
