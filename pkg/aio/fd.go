package aio

import (
	"net"
	"syscall"
)

type Fd interface {
	Fd() int
	ZeroReadIsEOF() bool
}

type NetFd interface {
	Fd
	Network() string
	Family() int
	SocketType() int
	Protocol() int
	IPv6Only() bool
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
}

func newNetFd(handle int, network string, family int, socketType int, protocol int, ipv6only bool, localAddr net.Addr, remoteAddr net.Addr) *netFd {
	return &netFd{
		handle:     handle,
		network:    network,
		family:     family,
		socketType: socketType,
		protocol:   protocol,
		ipv6only:   ipv6only,
		localAddr:  localAddr,
		remoteAddr: remoteAddr,
	}
}

type netFd struct {
	handle     int
	network    string
	family     int
	socketType int
	protocol   int
	ipv6only   bool
	localAddr  net.Addr
	remoteAddr net.Addr
}

func (s *netFd) Fd() int {
	return s.handle
}

func (s *netFd) Network() string {
	return s.network
}

func (s *netFd) Family() int {
	return s.family
}

func (s *netFd) SocketType() int {
	return s.socketType
}

func (s *netFd) Protocol() int {
	return s.protocol
}

func (s *netFd) IPv6Only() bool {
	return s.ipv6only
}

func (s *netFd) ZeroReadIsEOF() bool {
	return s.socketType != syscall.SOCK_DGRAM && s.socketType != syscall.SOCK_RAW
}

func (s *netFd) LocalAddr() net.Addr {
	return s.localAddr
}

func (s *netFd) RemoteAddr() net.Addr {
	return s.remoteAddr
}
