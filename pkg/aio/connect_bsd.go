//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

import "net"

func connect(network string, family int, sotype int, proto int, ipv6only bool, raddr net.Addr, laddr net.Addr, cb OperationCallback) {

	return
}
