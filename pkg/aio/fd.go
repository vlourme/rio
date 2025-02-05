package aio

import (
	"net"
	"sync/atomic"
	"syscall"
)

type Fd interface {
	Fd() int
	Reading() *Operator
	Writing() *Operator
	ReadOperator() *Operator
	WriteOperator() *Operator
	ZeroReadIsEOF() bool

	prepareReading() (op *Operator)
	finishReading()
	prepareWriting() (op *Operator)
	finishWriting()
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
		reading:    atomic.Bool{},
		writing:    atomic.Bool{},
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
	reading    atomic.Bool
	rop        *Operator
	writing    atomic.Bool
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

func (s *netFd) Reading() *Operator {
	if s.reading.Load() {
		return s.rop
	}
	return nil
}

func (s *netFd) Writing() *Operator {
	if s.writing.Load() {
		return s.wop
	}
	return nil
}

func (s *netFd) prepareReading() (op *Operator) {
	if s.reading.CompareAndSwap(false, true) {
		op = acquireOperator()
		op.setFd(s)
		s.rop = op
	}
	return
}

func (s *netFd) finishReading() {
	if s.reading.CompareAndSwap(true, false) {
		op := s.rop
		s.rop = nil
		releaseOperator(op)
	}
}

func (s *netFd) prepareWriting() (op *Operator) {
	if s.writing.CompareAndSwap(false, true) {
		op = acquireOperator()
		op.setFd(s)
		s.wop = op
	}
	return
}

func (s *netFd) finishWriting() {
	if s.writing.CompareAndSwap(true, false) {
		op := s.wop
		s.wop = nil
		releaseOperator(op)
	}
}

func (s *netFd) ReadOperator() *Operator {
	return s.rop
}

func (s *netFd) WriteOperator() *Operator {
	return s.wop
}
