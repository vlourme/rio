package sockets

import (
	"errors"
	"net"
	"net/netip"
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

type TCPAcceptHandler func(conn TCPConnection, err error)

type TCPListener interface {
	Addr() (addr net.Addr)
	Accept(handler TCPAcceptHandler)
	Close() (err error)
}

type TCPDialHandler func(conn TCPConnection, err error)

type TCPConnection interface {
	Connection
	SetNoDelay(noDelay bool) (err error)
	SetLinger(sec int) (err error)
	SetKeepAlive(keepalive bool) (err error)
	SetKeepAlivePeriod(period time.Duration) (err error)
}

type ReadFromHandler func(n int, addr net.Addr, err error)

type PacketConnection interface {
	Connection
	ReadFrom(p []byte, handler ReadFromHandler)
	WriteTo(p []byte, addr net.Addr, handler WriteHandler)
}

type ReadFromUDPHandler func(n int, addr *net.UDPAddr, err error)
type ReadFromUDPAddrPortHandler func(n int, addr netip.AddrPort, err error)
type ReadMsgUDPHandler func(n int, oobn int, flags int, addr *net.UDPAddr, err error)
type ReadMsgUDPAddrPortHandler func(n int, oobn int, flags int, addr netip.AddrPort, err error)
type WriteMsgHandler func(n int, oobn int, err error)

type UDPConnection interface {
	PacketConnection
	ReadMsgUDP(p []byte, oob []byte, handler ReadMsgUDPHandler)
	WriteMsgUDP(p []byte, oob []byte, addr *net.UDPAddr, handler WriteMsgHandler)
}

type ReadFromUnixHandler func(n int, addr *net.UnixAddr, err error)
type ReadMsgUnixHandler func(n int, oob []byte, flags int, addr *net.UnixAddr, err error)

type UnixConnection interface {
	Connection
	PacketConnection
	// ReadFromUnix acts like [UnixConn.ReadFrom] but returns a [UnixAddr].
	ReadFromUnix(p []byte, handler ReadFromUnixHandler)
	// ReadMsgUnix reads a message from c, copying the payload into b and
	// the associated out-of-band data into oob. It returns the number of
	// bytes copied into b, the number of bytes copied into oob, the flags
	// that were set on the message and the source address of the message.
	//
	// Note that if len(b) == 0 and len(oob) > 0, this function will still
	// read (and discard) 1 byte from the connection.
	ReadMsgUnix(p []byte, handler ReadMsgUnixHandler)
	// WriteToUnix acts like [UnixConn.WriteTo] but takes a [UnixAddr].
	WriteToUnix(p []byte, addr *net.UnixAddr, handler WriteHandler)
	// WriteMsgUnix writes a message to addr via c, copying the payload
	// from b and the associated out-of-band data from oob. It returns the
	// number of payload and out-of-band bytes written.
	//
	// Note that if len(b) == 0 and len(oob) > 0, this function will still
	// write 1 byte to the connection.
	WriteMsgUnix(b, oob []byte, addr *net.UnixAddr, handler WriteMsgHandler)
}

type UnixDialHandler func(conn UnixConnection, err error)

type UnixAcceptHandler func(conn UnixConnection, err error)

type UnixListener interface {
	Addr() (addr net.Addr)
	AcceptUnix(handler UnixAcceptHandler)
	Close() (err error)
}

type IPConnection interface {
}
