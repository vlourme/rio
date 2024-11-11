package sockets

import (
	"errors"
	"net"
)

func ListenUPD(network string, address string, _ Options) (conn UDPConnection, err error) {
	addr, family, ipv6only, addrErr := GetAddrAndFamily(network, address)
	if addrErr != nil {
		err = &net.OpError{Op: "listen", Net: network, Source: nil, Addr: nil, Err: addrErr}
		return
	}
	udpAddr, isUDPAddr := addr.(*net.UDPAddr)
	if !isUDPAddr {
		err = &net.OpError{Op: "listen", Net: network, Source: nil, Addr: nil, Err: errors.New("not a UDP address")}
		return
	}
	conn, err = listenUDP(network, family, udpAddr, ipv6only, 0)
	return
}
