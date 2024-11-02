package sockets

import (
	"errors"
	"net"
)

func ListenTCP(network, address string, opt Options) (ln Listener, err error) {
	addr, family, ipv6only, addrErr := GetAddrAndFamily(network, address)
	if addrErr != nil {
		err = &net.OpError{Op: "listen", Net: network, Source: nil, Addr: nil, Err: addrErr}
		return
	}
	tcpAddr, isTCPAddr := addr.(*net.TCPAddr)
	if !isTCPAddr {
		err = &net.OpError{Op: "listen", Net: network, Source: nil, Addr: nil, Err: errors.New("not a TCP address")}
		return
	}
	ln, err = newTCPListener(network, family, tcpAddr, ipv6only, opt.Proto, opt.Pollers)
	return
}

func DialTCP() (conn Connection, err error) {

	return
}
