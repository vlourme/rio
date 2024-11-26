package aio

import (
	"errors"
	"net"
)

type ListenerOptions struct {
	MultipathTCP       bool
	MulticastInterface *net.Interface
}

func Listen(network string, address string, opts ListenerOptions) (fd NetFd, err error) {
	switch network {
	case "tcp", "tcp4", "tcp6":
		break
	case "unix", "unixpacket":
		break
	default:
		err = errors.New("aio.Listen: network is not support")
		return
	}

	addr, family, _, addrErr := ResolveAddr(network, address)
	if addrErr != nil {
		err = &net.OpError{Op: "listen", Net: network, Source: nil, Addr: nil, Err: addrErr}
		return
	}
	proto := 0
	if opts.MultipathTCP {
		proto = tryGetMultipathTCPProto()
	}
	fd, err = newListener(network, family, addr, proto)
	return
}
