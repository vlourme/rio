package sockets

import (
	"errors"
	"net"
)

func ListenTCP(network string, address string, opt Options) (ln TCPListener, err error) {
	addr, family, ipv6only, addrErr := GetAddrAndFamily(network, address)
	if addrErr != nil {
		err = &net.OpError{Op: "listen", Net: network, Source: nil, Addr: nil, Err: addrErr}
		return
	}
	tcpAddr, isTCPAddr := addr.(*net.TCPAddr)
	if !isTCPAddr {
		err = &net.OpError{Op: "listen", Net: network, Source: nil, Addr: nil, Err: errors.New("sockets: not a TCP address")}
		return
	}
	proto := 0
	if opt.MultipathTCP {
		proto = tryGetMultipathTCPProto()
	}
	ln, err = newTCPListener(network, family, tcpAddr, ipv6only, proto)
	return
}

func DialTCP(network string, address string, opt Options, handler TCPDialHandler) {
	addr, family, ipv6only, addrErr := GetAddrAndFamily(network, address)
	if addrErr != nil {
		handler(nil, &net.OpError{Op: "dial", Net: network, Source: nil, Addr: nil, Err: addrErr})
		return
	}
	tcpAddr, isTCPAddr := addr.(*net.TCPAddr)
	if !isTCPAddr {
		handler(nil, &net.OpError{Op: "dial", Net: network, Source: nil, Addr: nil, Err: errors.New("sockets: not a TCP address")})
		return
	}
	proto := 0
	if opt.MultipathTCP {
		proto = tryGetMultipathTCPProto()
	}
	connectTCP(network, family, tcpAddr, ipv6only, proto, handler)
	return
}
