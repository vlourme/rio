//go:build windows

package sockets

import (
	"golang.org/x/sys/windows"
	"net"
	"net/netip"
)

func listenUDP(network string, family int, addr *net.UDPAddr, ipv6only bool, proto int, handler ListenUDPHandler) {
	// conn
	// iocp
	// new udp conn
	// todo or use post status with op(mode = listen udp)
}

func newUDPConnection(network string, family int, addr *net.UDPAddr, ipv6only bool, proto int) (uc *udpConnection, err error) {
	// conn
	conn, connErr := newConnection(network, family, windows.SOCK_DGRAM, proto, ipv6only)
	if connErr != nil {
		err = connErr
		return
	}
	// bind
	lsa := addrToSockaddr(family, addr)
	bindErr := windows.Bind(conn.fd, lsa)
	if bindErr != nil {
		err = bindErr
		_ = windows.Closesocket(conn.fd)
		return
	}
	// CreateIoCompletionPort
	cphandle, createErr := windows.CreateIoCompletionPort(conn.fd, iocp, key, 0)
	if createErr != nil {
		_ = windows.Closesocket(conn.fd)
		err = createErr
		return
	}
	conn.cphandle = cphandle
	// udp
	uc = &udpConnection{
		connection: *conn,
	}
	return
}

type udpConnection struct {
	connection
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
