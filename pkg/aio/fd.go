package aio

import "net"

type Fd interface {
	Fd() int
	ReadOperator() Operator
	WriteOperator() Operator
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

type SocketFd struct {
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

func (s *SocketFd) Fd() int {
	return s.handle
}

func (s *SocketFd) Network() string {
	return s.network
}

func (s *SocketFd) Family() int {
	return s.family
}

func (s *SocketFd) SocketType() int {
	return s.socketType
}

func (s *SocketFd) Protocol() int {
	return s.protocol
}

func (s *SocketFd) LocalAddr() net.Addr {
	return s.localAddr
}

func (s *SocketFd) RemoteAddr() net.Addr {
	return s.remoteAddr
}

func (s *SocketFd) ReadOperator() Operator {
	return s.rop
}

func (s *SocketFd) WriteOperator() Operator {
	return s.wop
}
