package sockets

import (
	"errors"
	"net"
	"time"
)

var (
	ErrEmptyPacket = errors.New("sockets: empty packet")
)

type ReadHandler func(n int, err error)
type WriteHandler func(n int, err error)

type Connection interface {
	LocalAddr() (addr net.Addr)
	RemoteAddr() (addr net.Addr)
	SetDeadline(deadline time.Time) (err error)
	SetReadDeadline(deadline time.Time) (err error)
	SetWriteDeadline(deadline time.Time) (err error)
	Read(p []byte, handler ReadHandler)
	Write(p []byte, handler WriteHandler)
	Close() (err error)
}

// *********************************************************************************************************************

type TCPConnection interface {
	Connection
	SetNoDelay(noDelay bool) (err error)
	SetLinger(sec int) (err error)
	SetKeepAlive(keepalive bool) (err error)
	SetKeepAlivePeriod(period time.Duration) (err error)
}

// *********************************************************************************************************************

type AcceptHandler func(conn Connection, err error)

type Listener interface {
	Addr() (addr net.Addr)
	Accept(handler AcceptHandler)
	Close() (err error)
}

// *********************************************************************************************************************

type ReadFromHandler func(n int, addr net.Addr, err error)
type ReadMsgHandler func(n int, oobn int, flags int, addr net.Addr, err error)
type WriteMsgHandler func(n int, oobn int, err error)

type PacketConnection interface {
	Connection
	ReadFrom(p []byte, handler ReadFromHandler)
	WriteTo(p []byte, addr net.Addr, handler WriteHandler)
	ReadMsg(p []byte, oob []byte, handler ReadMsgHandler)
	WriteMsg(p []byte, oob []byte, addr net.Addr, handler WriteMsgHandler)
}

// *********************************************************************************************************************

type DialHandler func(conn Connection, err error)

func Dial(network string, address string, opt Options, handler DialHandler) {
	addr, family, ipv6only, addrErr := GetAddrAndFamily(network, address)
	if addrErr != nil {
		handler(nil, &net.OpError{Op: "dial", Net: network, Source: nil, Addr: nil, Err: addrErr})
		return
	}
	proto := 0
	switch network {
	case "tcp", "tcp4", "tcp6":
		if opt.MultipathTCP {
			proto = tryGetMultipathTCPProto()
		}
		break
	default:
		break
	}
	connect(network, family, addr, ipv6only, proto, handler)
	return
}
