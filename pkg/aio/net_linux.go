//go:build linux

package aio

import (
	"net"
)

func newNetFd(network string, family int, sotype int, proto int, laddr net.Addr, raddr net.Addr, multicastInterface *net.Interface) (v *netFd, err error) {

	return
}
