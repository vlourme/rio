//go:build !linux

package rio

import "net"

func Listen(network string, addr string) (net.Listener, error) {
	return net.Listen(network, addr)
}
