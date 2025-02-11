package aio

import (
	"net"
	"sync/atomic"
	"syscall"
	"unsafe"
)

type Fd interface {
	Fd() int
	ZeroReadIsEOF() bool
	Cylinder() Cylinder
	ROP() *Operator
	SetROP(op *Operator) bool
	RemoveROP()
	WOP() *Operator
	SetWOP(op *Operator) bool
	RemoveWOP()
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

func newNetFd(cylinder Cylinder, handle int, network string, family int, socketType int, protocol int, ipv6only bool, localAddr net.Addr, remoteAddr net.Addr) *netFd {
	return &netFd{
		cylinder:   cylinder,
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
	cylinder   Cylinder
	handle     int
	network    string
	family     int
	socketType int
	protocol   int
	ipv6only   bool
	localAddr  net.Addr
	remoteAddr net.Addr
	rop        atomic.Uintptr
	wop        atomic.Uintptr
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

func (s *netFd) Cylinder() Cylinder {
	return s.cylinder
}

func (s *netFd) ROP() *Operator {
	if ptr := s.rop.Load(); ptr > 0 {
		return (*Operator)(unsafe.Pointer(ptr))
	}
	return nil
}

func (s *netFd) SetROP(op *Operator) bool {
	ptr := uintptr(unsafe.Pointer(op))
	return s.rop.CompareAndSwap(0, ptr)
}

func (s *netFd) RemoveROP() {
	s.rop.Store(0)
}

func (s *netFd) WOP() *Operator {
	if ptr := s.wop.Load(); ptr > 0 {
		return (*Operator)(unsafe.Pointer(ptr))
	}
	return nil
}

func (s *netFd) SetWOP(op *Operator) bool {
	ptr := uintptr(unsafe.Pointer(op))
	return s.wop.CompareAndSwap(0, ptr)
}

func (s *netFd) RemoveWOP() {
	s.wop.Store(0)
}
