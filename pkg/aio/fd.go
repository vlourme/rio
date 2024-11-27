package aio

import (
	"net"
	"syscall"
)

type Fd interface {
	Fd() int
	ReadOperator() Operator
	WriteOperator() Operator
	ZeroReadIsEOF() bool
}

type NetFd interface {
	Fd
	Network() string
	Family() int
	SocketType() int
	Protocol() int
	LocalAddr() net.Addr
	RemoteAddr() net.Addr
}

type FileFd interface {
	Fd
	Path() string
}

type netFd struct {
	handle     int
	network    string
	family     int
	socketType int
	protocol   int
	localAddr  net.Addr
	remoteAddr net.Addr
	rop        Operator
	wop        Operator
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

func (s *netFd) ZeroReadIsEOF() bool {
	return s.socketType != syscall.SOCK_DGRAM && s.socketType != syscall.SOCK_RAW
}

func (s *netFd) LocalAddr() net.Addr {
	return s.localAddr
}

func (s *netFd) RemoteAddr() net.Addr {
	return s.remoteAddr
}

func (s *netFd) ReadOperator() Operator {
	return s.rop
}

func (s *netFd) WriteOperator() Operator {
	return s.wop
}
