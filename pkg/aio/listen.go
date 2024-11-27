package aio

import (
	"errors"
	"net"
	"syscall"
	"time"
)

type ListenerOptions struct {
	MultipathTCP       bool
	MulticastInterface *net.Interface
}

func Listen(network string, address string, opts ListenerOptions) (fd NetFd, err error) {
	addr, family, _, addrErr := ResolveAddr(network, address)
	if addrErr != nil {
		err = addrErr
		return
	}

	switch network {
	case "tcp", "tcp4", "tcp6":
		proto := 0
		if opts.MultipathTCP {
			proto = tryGetMultipathTCPProto()
		}
		fd, err = newListenerFd(network, family, syscall.SOCK_STREAM, proto, addr, nil)
		break
	case "udp", "udp4", "udp6":
		fd, err = newListenerFd(network, family, syscall.SOCK_DGRAM, 0, addr, opts.MulticastInterface)
		break
	case "unix":
		fd, err = newListenerFd(network, family, syscall.SOCK_STREAM, 0, addr, nil)
		break
	case "unixgram":
		fd, err = newListenerFd(network, family, syscall.SOCK_DGRAM, 0, addr, nil)
		break
	case "unixpacket":
		fd, err = newListenerFd(network, family, syscall.SOCK_SEQPACKET, 0, addr, nil)
		break
	case "ip", "ip4", "ip6":
		proto := 0
		var parseProtoError error
		network, proto, parseProtoError = ParseIpProto(network)
		if parseProtoError != nil {
			err = parseProtoError
			return
		}
		fd, err = newListenerFd(network, family, syscall.SOCK_RAW, proto, addr, nil)
		break
	default:
		err = errors.New("aio.Listen: network is not support")
		return
	}
	return
}

const (
	defaultTCPKeepAliveIdle = 5 * time.Second
)

func roundDurationUp(d time.Duration, to time.Duration) time.Duration {
	return (d + to - 1) / to
}
