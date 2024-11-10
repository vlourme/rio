//go:build windows

package sockets

import (
	"golang.org/x/sys/windows"
	"net"
	"net/netip"
)

func newUDPConnection(network string, family int, addr *net.UDPAddr, ipv6only bool, proto int, pollers int) (conn *udpConnection, err error) {
	conn = &udpConnection{
		connection: connection{},
		cphandle:   0,
		fd:         0,
		addr:       nil,
		family:     0,
		net:        "",
	}
	return
}

type udpConnection struct {
	connection
	cphandle windows.Handle
	fd       windows.Handle
	addr     *net.TCPAddr
	family   int
	net      string
}

func (conn *udpConnection) ReadFrom(p []byte, handler ReadFromHandler) {
	//TODO implement me
	panic("implement me")
}

func (conn *udpConnection) ReadFromUDP(p []byte, handler ReadFromUDPHandler) {
	//TODO implement me
	panic("implement me")
}

func (conn *udpConnection) ReadFromUDPAddrPort(p []byte, handler ReadFromUDPAddrPortHandler) {
	//TODO implement me
	panic("implement me")
}

func (conn *udpConnection) WriteTo(p []byte, addr net.Addr, handler WriteHandler) {
	//TODO implement me
	panic("implement me")
}

func (conn *udpConnection) WriteToUDP(b []byte, addr *net.UDPAddr, handler WriteHandler) {
	//TODO implement me
	panic("implement me")
}

func (conn *udpConnection) WriteToUDPAddrPort(b []byte, addr netip.AddrPort, handler WriteHandler) {
	//TODO implement me
	panic("implement me")
}

func (conn *udpConnection) ReadMsgUDP(p []byte, oob []byte, handler ReadMsgUDPHandler) {
	//TODO implement me
	panic("implement me")
}

func (conn *udpConnection) ReadMsgUDPAddrPort(b, oob []byte, handler ReadMsgUDPAddrPortHandler) {
	//TODO implement me
	panic("implement me")
}

func (conn *udpConnection) WriteMsgUDP(b, oob []byte, addr *net.UDPAddr, handler WriteMsgHandler) {
	//TODO implement me
	panic("implement me")
}

func (conn *udpConnection) WriteMsgUDPAddrPort(b, oob []byte, addr netip.AddrPort, handler WriteMsgHandler) {
	//TODO implement me
	panic("implement me")
}

func (conn *udpConnection) Close() (err error) {
	// close socket
	err = conn.connection.Close()
	// todo close iocp ?
	_ = windows.CloseHandle(conn.cphandle)
	return
}
