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
	addr, family, _, addrErr := ResolveAddr(network, address)
	if addrErr != nil {
		err = &net.OpError{Op: "listen", Net: network, Source: nil, Addr: nil, Err: addrErr}
		return
	}

	switch network {
	case "tcp", "tcp4", "tcp6":
		proto := 0
		if opts.MultipathTCP {
			proto = tryGetMultipathTCPProto()
		}
		fd, err = newNetFd(network, family, syscall.SOCK_STREAM, proto, addr, nil, nil)
		//fd, err = newListener(network, family, addr, proto)
		break
	case "udp", "udp4", "udp6":
		fd, err = newNetFd(network, family, syscall.SOCK_DGRAM, 0, addr, nil, opts.MulticastInterface)
		break
	case "unix":
		fd, err = newNetFd(network, family, syscall.SOCK_STREAM, 0, addr, nil, nil)
		break
	case "unixgram":
		fd, err = newNetFd(network, family, syscall.SOCK_DGRAM, 0, addr, nil, nil)
		break
	case "unixpacket":
		fd, err = newNetFd(network, family, syscall.SOCK_SEQPACKET, 0, addr, nil, nil)
		break
	case "ip", "ip4", "ip6":
		break
	default:
		err = errors.New("aio.Listen: network is not support")
		return
	}
	return
}
