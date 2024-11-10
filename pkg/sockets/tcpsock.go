package sockets

import (
	"errors"
	"net"
	"time"
)

func ListenTCP(network string, address string, opt Options) (ln TCPListener, err error) {
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
	ln, err = newTCPListener(network, family, tcpAddr, ipv6only, opt.Proto)
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
		handler(nil, &net.OpError{Op: "dial", Net: network, Source: nil, Addr: nil, Err: errors.New("not a TCP address")})
		return
	}
	connectTCP(network, family, tcpAddr, ipv6only, opt.Proto, handler)
	return
}

// roundDurationUp rounds d to the next multiple of to.
func roundDurationUp(d time.Duration, to time.Duration) time.Duration {
	return (d + to - 1) / to
}
