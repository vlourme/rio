package rio

import (
	"github.com/brickingsoft/rio/pkg/sys"
	"net"
)

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
