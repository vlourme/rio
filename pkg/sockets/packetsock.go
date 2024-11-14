package sockets

import (
	"errors"
	"net"
)

func ListenPacket(network string, address string, _ Options) (conn PacketConnection, err error) {
	switch network {
	case "udp", "udp4", "udp6":
		break
	case "unixgram":
		break
	case "ip":
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
	conn, err = newPacketConnection(network, family, addr, ipv6only, 0)
	return
}
