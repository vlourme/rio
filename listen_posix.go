//go:build !linux

package rio

import "net"

func Listen(network string, addr string) (net.Listener, error) {
	ln, lnErr := net.Listen(network, addr)
	if lnErr != nil {
		return nil, lnErr
	}
	switch v := ln.(type) {
	case *net.TCPListener:
		return &TCPListener{v}, nil
	case *net.UnixListener:
		return &UnixListener{v}, nil
	default:
		return ln, nil
	}
}
