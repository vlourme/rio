package rio

import (
	"github.com/brickingsoft/rio/pkg/sys"
	"net"
)

func ResolveTCPAddr(address string) (*net.TCPAddr, error) {
	addr, _, _, err := sys.ResolveAddr("tcp", address)
	if err != nil {
		return nil, err
	}
	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok {
		return nil, err
	}
	return tcpAddr, nil
}

func ResolveUDPAddr(address string) (*net.UDPAddr, error) {
	addr, _, _, err := sys.ResolveAddr("udp", address)
	if err != nil {
		return nil, err
	}
	udpAddr, ok := addr.(*net.UDPAddr)
	if !ok {
		return nil, err
	}
	return udpAddr, nil
}

func ResolveUnixAddr(address string) (*net.UnixAddr, error) {
	addr, _, _, err := sys.ResolveAddr("unix", address)
	if err != nil {
		return nil, err
	}
	unixAddr, ok := addr.(*net.UnixAddr)
	if !ok {
		return nil, err
	}
	return unixAddr, nil
}

func ResolveIPAddr(address string) (*net.IPAddr, error) {
	addr, _, _, err := sys.ResolveAddr("ip", address)
	if err != nil {
		return nil, err
	}
	ipAddr, ok := addr.(*net.IPAddr)
	if !ok {
		return nil, err
	}
	return ipAddr, nil
}
