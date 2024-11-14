package sockets

import (
	"errors"
	"net"
)

func Listen(network string, address string, opt Options) (ln Listener, err error) {
	switch network {
	case "tcp", "tcp4", "tcp6":
		break
	case "unix", "unixpacket":
		break
	default:
		err = &net.OpError{Op: "listen", Net: network, Source: nil, Addr: nil, Err: errors.New("sockets: network is not support")}
		return
	}

	addr, family, ipv6only, addrErr := GetAddrAndFamily(network, address)
	if addrErr != nil {
		err = &net.OpError{Op: "listen", Net: network, Source: nil, Addr: nil, Err: addrErr}
		return
	}

	proto := 0
	if opt.MultipathTCP {
		proto = tryGetMultipathTCPProto()
	}
	ln, err = newListener(network, family, addr, ipv6only, proto)
	return
}
