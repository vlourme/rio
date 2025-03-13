package tls

import (
	"crypto/tls"
	"github.com/brickingsoft/rio"
	"net"
)

func DialWithDialer(dialer *rio.Dialer, network, addr string, config *tls.Config) (*tls.Conn, error) {
	d := net.Dialer{
		Timeout:         dialer.Timeout,
		Deadline:        dialer.Deadline,
		LocalAddr:       dialer.LocalAddr,
		DualStack:       false,
		FallbackDelay:   0,
		KeepAlive:       dialer.KeepAlive,
		KeepAliveConfig: dialer.KeepAliveConfig,
		Resolver:        nil,
		Cancel:          nil,
		Control:         dialer.Control,
		ControlContext:  dialer.ControlContext,
	}
	return tls.DialWithDialer(&d, network, addr, config)
}

func Dial(network, addr string, config *tls.Config) (*tls.Conn, error) {
	return DialWithDialer(&rio.DefaultDialer, network, addr, config)
}
