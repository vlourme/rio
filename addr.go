package rio

import (
	"fmt"
	"github.com/brickingsoft/errors"
	"github.com/brickingsoft/rio/pkg/aio"
	"net"
)

func ResolveUDPAddr(address string) (*net.UDPAddr, error) {
	addr, _, _, err := aio.ResolveAddr("udp", address)
	if err != nil {
		return nil, errors.New("resolve udp addr fail", errors.WithMeta(errMetaPkgKey, errMetaPkgVal), errors.WithWrap(err))
	}
	udpAddr, ok := addr.(*net.UDPAddr)
	if !ok {
		return nil, errors.New("resolve udp addr fail", errors.WithMeta(errMetaPkgKey, errMetaPkgVal), errors.WithWrap(errors.Define(fmt.Sprintf(" %s is not a UDP address", address))))
	}
	return udpAddr, nil
}
