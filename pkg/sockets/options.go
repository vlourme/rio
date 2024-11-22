package sockets

import "net"

type Options struct {
	MultipathTCP            bool
	DialPacketConnLocalAddr net.Addr
	MulticastInterface      *net.Interface
}
