//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

import (
	"net"
)

func newListenerFd(network string, family int, sotype int, proto int, addr net.Addr, multicastInterface *net.Interface) (v *netFd, err error) {

	return
}

func Accept(fd NetFd, cb OperationCallback) {

}
