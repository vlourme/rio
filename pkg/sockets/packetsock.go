package sockets

import (
	"errors"
	"golang.org/x/sys/windows"
	"net"
)

func ListenPacket(network string, address string, opt Options) (conn PacketConnection, err error) {
	sotype := 0
	switch network {
	case "udp", "udp4", "udp6":
		sotype = windows.SOCK_DGRAM
		break
	case "unixgram":
		sotype = windows.SOCK_DGRAM
		break
	case "ip", "ip4", "ip6":
		sotype = windows.SOCK_RAW
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
	conn, err = newPacketConnection(network, family, sotype, addr, nil, ipv6only, 0, opt.MulticastInterface)
	return
}
