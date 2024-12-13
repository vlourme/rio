package aio

import (
	"errors"
	"net"
	"syscall"
)

type ListenerOptions struct {
	MultipathTCP       bool
	MulticastInterface *net.Interface
}

func Listen(network string, address string, opts ListenerOptions) (fd NetFd, err error) {
	addr, family, ipv6only, addrErr := ResolveAddr(network, address)
	if addrErr != nil {
		err = addrErr
		return
	}

	switch network {
	case "tcp", "tcp4", "tcp6":
		proto := syscall.IPPROTO_TCP
		if opts.MultipathTCP {
			proto = tryGetMultipathTCPProto()
		}
		fd, err = newListenerFd(network, family, syscall.SOCK_STREAM, proto, ipv6only, addr, nil)
		break
	case "udp", "udp4", "udp6":
		fd, err = newListenerFd(network, family, syscall.SOCK_DGRAM, syscall.IPPROTO_UDP, ipv6only, addr, opts.MulticastInterface)
		break
	case "unix":
		fd, err = newListenerFd(network, family, syscall.SOCK_STREAM, 0, ipv6only, addr, nil)
		break
	case "unixgram":
		fd, err = newListenerFd(network, family, syscall.SOCK_DGRAM, 0, ipv6only, addr, nil)
		break
	case "unixpacket":
		fd, err = newListenerFd(network, family, syscall.SOCK_SEQPACKET, 0, ipv6only, addr, nil)
		break
	case "ip", "ip4", "ip6":
		proto := 0
		var parseProtoError error
		network, proto, parseProtoError = ParseIpProto(network)
		if parseProtoError != nil {
			err = parseProtoError
			return
		}
		fd, err = newListenerFd(network, family, syscall.SOCK_RAW, proto, ipv6only, addr, nil)
		break
	default:
		err = errors.New("aio.Listen: network is not support")
		return
	}
	return
}
