package aio

import (
	"net"
	"syscall"
)

type Fd interface {
	Fd() int
	ReadOperator() *Operator
	WriteOperator() *Operator
	ZeroReadIsEOF() bool
}

type FileFd interface {
	Fd
	Path() string
}

type fileFd struct {
	handle int
	path   string
	rop    *Operator
	wop    *Operator
}

func (fd *fileFd) Fd() int {
	return fd.handle
}

func (fd *fileFd) ReadOperator() *Operator {
	return fd.rop
}

func (fd *fileFd) WriteOperator() *Operator {
	return fd.wop
}

func (fd *fileFd) ZeroReadIsEOF() bool {
	return true
}

func (fd *fileFd) Path() string {
	return fd.path
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

type netFd struct {
	handle     int
	network    string
	family     int
	socketType int
	protocol   int
	ipv6only   bool
	localAddr  net.Addr
	remoteAddr net.Addr
	rop        *Operator
	wop        *Operator
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

func (s *netFd) ReadOperator() *Operator {
	return s.rop
}

func (s *netFd) WriteOperator() *Operator {
	return s.wop
}
