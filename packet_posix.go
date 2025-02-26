//go:build !linux

package rio

import (
	"net"
)

func ListenPacket(network string, addr string) (net.PacketConn, error) {
	return net.ListenPacket(network, addr)
}
