//go:build windows

package sockets

import "net"

type udpListener struct{}

type udpConnection struct {
	connection
}

func (conn *udpConnection) ReadFrom(p []byte, handler ReadFromHandler) {
	//TODO implement me
	panic("implement me")
}

func (conn *udpConnection) WriteTo(p []byte, addr net.Addr, handler WriteHandler) {
	//TODO implement me
	panic("implement me")
}
