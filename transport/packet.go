package transport

import (
	"github.com/brickingsoft/rxp/async"
	"net"
)

type PacketInbound interface {
	Reader() (r InboundReader)
	Received() (n int)
	Addr() (addr net.Addr)
}

func NewPacketInbound(r InboundReader, addr net.Addr, n int) PacketInbound {
	return &packetInbound{
		r: r,
		a: addr,
		n: n,
	}
}

type packetInbound struct {
	r InboundReader
	a net.Addr
	n int
}

func (in *packetInbound) Reader() (r InboundReader) {
	r = in.r
	return
}

func (in *packetInbound) Received() (n int) {
	n = in.n
	return
}

func (in *packetInbound) Addr() (addr net.Addr) {
	addr = in.a
	return
}

type PacketMsgInbound interface {
	Reader() (r InboundReader)
	Received() (n int)
	OOB() (r InboundReader)
	OOReceived() (n int)
	Flags() (n int)
	Addr() (addr net.Addr)
}

func NewPacketMsgInbound(r InboundReader, oob InboundReader, addr net.Addr, n int, oobn int, flags int) PacketMsgInbound {
	return &packetMsgInbound{
		r: r,
		o: oob,
		a: addr,
		f: flags,
		n: n,
		m: oobn,
	}
}

type packetMsgInbound struct {
	r InboundReader
	o InboundReader
	a net.Addr
	f int
	n int
	m int
}

func (in *packetMsgInbound) Reader() (r InboundReader) {
	r = in.r
	return
}

func (in *packetMsgInbound) Received() (n int) {
	n = in.n
	return
}

func (in *packetMsgInbound) OOB() (r InboundReader) {
	r = in.o
	return
}

func (in *packetMsgInbound) OOReceived() (n int) {
	n = in.m
	return
}

func (in *packetMsgInbound) Flags() (n int) {
	n = in.f
	return
}

func (in *packetMsgInbound) Addr() (addr net.Addr) {
	addr = in.a
	return
}

type PacketMsgOutbound interface {
	Written() (n int)
	OOBWrote() (n int)
	UnexpectedError() (err error)
}

func NewPacketMsgOutbound(n int, oobn int, unexpectedError error) PacketMsgOutbound {
	return &packetMsgOutbound{
		wrote:           n,
		oobWrote:        oobn,
		unexpectedError: unexpectedError,
	}
}

type packetMsgOutbound struct {
	wrote           int
	oobWrote        int
	unexpectedError error
}

func (out *packetMsgOutbound) Written() (n int) {
	n = out.wrote
	return
}

func (out *packetMsgOutbound) OOBWrote() (n int) {
	n = out.oobWrote
	return
}

func (out *packetMsgOutbound) UnexpectedError() (err error) {
	err = out.unexpectedError
	return
}

type PacketReader interface {
	ReadFrom() (future async.Future[PacketInbound])
}

type PacketWriter interface {
	WriteTo(b []byte, addr net.Addr) (future async.Future[int])
}
