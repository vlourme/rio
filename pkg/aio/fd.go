package aio

import (
	"net"
	"syscall"
	"time"
)

type Fd interface {
	Fd() int
	ReadOperator() Operator
	WriteOperator() Operator
	SetOperatorTimeout(d time.Duration)
	SetReadTimeout(d time.Duration)
	ReadTimeout() time.Duration
	SetWriteTimeout(d time.Duration)
	WriteTimeout() time.Duration
	ZeroReadIsEOF() bool
}

type FileFd interface {
	Fd
	Path() string
}

type fileFd struct {
	handle int
	path   string
	rop    Operator
	wop    Operator
}

func (fd *fileFd) Fd() int {
	return fd.handle
}

func (fd *fileFd) ReadOperator() Operator {
	return fd.rop
}

func (fd *fileFd) WriteOperator() Operator {
	return fd.wop
}

func (fd *fileFd) SetOperatorTimeout(d time.Duration) {
	fd.SetReadTimeout(d)
	fd.SetWriteTimeout(d)
}

func (fd *fileFd) SetReadTimeout(d time.Duration) {
	if d > 0 {
		fd.rop.timeout = d
	}
}

func (fd *fileFd) ReadTimeout() time.Duration {
	return fd.rop.timeout
}

func (fd *fileFd) SetWriteTimeout(d time.Duration) {
	if d > 0 {
		fd.wop.timeout = d
	}
}

func (fd *fileFd) WriteTimeout() time.Duration {
	return fd.wop.timeout
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

func (s *netFd) ReadOperator() Operator {
	return s.rop
}

func (s *netFd) WriteOperator() Operator {
	return s.wop
}

func (s *netFd) SetOperatorTimeout(d time.Duration) {
	s.SetReadTimeout(d)
	s.SetWriteTimeout(d)
}

func (s *netFd) SetReadTimeout(d time.Duration) {
	if d > 0 {
		s.rop.timeout = d
	}
}

func (s *netFd) ReadTimeout() time.Duration {
	return s.rop.timeout
}

func (s *netFd) SetWriteTimeout(d time.Duration) {
	if d > 0 {
		s.wop.timeout = d
	}
}

func (s *netFd) WriteTimeout() time.Duration {
	return s.wop.timeout
}
