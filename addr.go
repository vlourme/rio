package rio

import (
	"fmt"
	"github.com/brickingsoft/rio/pkg/aio"
	"net"
)

func ResolveUDPAddr(address string) (*net.UDPAddr, error) {
	addr, _, _, err := aio.ResolveAddr("udp", address)
	if err != nil {
		return nil, err
	}
	udpAddr, ok := addr.(*net.UDPAddr)
	if !ok {
		return nil, fmt.Errorf("rio: %s is not a UDP address", address)
	}
	return udpAddr, nil
}
