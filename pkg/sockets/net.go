package sockets

import (
	"io"
	"net"
	"net/netip"
	"time"
)

type ReadHandler func(n int, err error)
type WriteHandler func(n int, err error)
type CloseHandler func(err error)

type Connection interface {
	LocalAddr() (addr net.Addr)
	RemoteAddr() (addr net.Addr)
	SetDeadline(t time.Time) (err error)
	SetReadDeadline(t time.Time) (err error)
	SetWriteDeadline(t time.Time) (err error)
	SetReadBuffer(bytes int) (err error)
	SetWriteBuffer(bytes int) (err error)
	Read(p []byte, handler ReadHandler) (err error)
	Write(p []byte, handler WriteHandler) (err error)
	Close(handler CloseHandler) (err error)
}

type AcceptHandler func(conn Connection, err error)

type Listener interface {
	Addr() (addr net.Addr)
	Accept(handler AcceptHandler)
	Close() (err error)
	polling()
}

type TCPConnection interface {
	Connection
	// ReadFrom
	// use sendfile or splice
	// for windows, sendfile is TransmitFile in iocp, so use event to block it.
	// when not supported, then use io.copy to copy r into one buf, then write it.
	ReadFrom(r io.Reader) (n int64, err error)
	// WriteTo
	// spliceTo
	// when not supported, then use io.copy tp copy conn.buf into w.
	WriteTo(w io.Writer) (n int64, err error)
	SetNoDelay(noDelay bool) (err error)
	SetLinger(sec int) (err error)
	SetKeepAlive(keepalive bool) (err error)
	SetKeepAlivePeriod(d time.Duration) (err error)
}

type ReadFromHandler func(n int, addr net.Addr, err error)

type PacketConnection interface {
	LocalAddr() (addr net.Addr)
	SetDeadline(t time.Time) (err error)
	SetReadDeadline(t time.Time) (err error)
	SetWriteDeadline(t time.Time) (err error)
	ReadFrom(p []byte, handler ReadFromHandler) (err error)
	WriteTo(p []byte, addr net.Addr, handler WriteHandler) (err error)
	Close(handler CloseHandler) (err error)
}

type ReadFromUDPHandler func(n int, addr *net.UDPAddr, err error)
type ReadFromUDPAddrPortHandler func(n int, addr netip.AddrPort, err error)
type ReadMsgUDPHandler func(n int, oobn int, flags int, addr *net.UDPAddr, err error)
type ReadMsgUDPAddrPortHandler func(n int, oobn int, flags int, addr netip.AddrPort, err error)
type WriteMsgHandler func(n int, oobn int, err error)

type UPDConnection interface {
	PacketConnection
	// ReadFromUDP acts like ReadFrom but returns a UDPAddr.
	ReadFromUDP(p []byte, handler ReadFromUDPHandler) (err error)
	// ReadFromUDPAddrPort acts like ReadFrom but returns a netip.AddrPort.
	//
	// If c is bound to an unspecified address, the returned
	// netip.AddrPort's address might be an IPv4-mapped IPv6 address.
	// Use netip.Addr.Unmap to get the address without the IPv6 prefix.
	ReadFromUDPAddrPort(p []byte, handler ReadFromUDPAddrPortHandler) (err error)
	// ReadMsgUDP reads a message from c, copying the payload into b and
	// the associated out-of-band data into oob. It returns the number of
	// bytes copied into b, the number of bytes copied into oob, the flags
	// that were set on the message and the source address of the message.
	//
	// The packages golang.org/x/net/ipv4 and golang.org/x/net/ipv6 can be
	// used to manipulate IP-level socket options in oob.
	ReadMsgUDP(p []byte, oob []byte, handler ReadMsgUDPHandler) (err error)
	// ReadMsgUDPAddrPort is like ReadMsgUDP but returns an netip.AddrPort instead of a UDPAddr.
	ReadMsgUDPAddrPort(b, oob []byte, handler ReadMsgUDPAddrPortHandler) (err error)
	// WriteToUDP acts like WriteTo but takes a UDPAddr.
	WriteToUDP(b []byte, addr *net.UDPAddr, handler WriteHandler) (err error)
	// WriteToUDPAddrPort acts like WriteTo but takes a netip.AddrPort.
	WriteToUDPAddrPort(b []byte, addr netip.AddrPort, handler WriteHandler) (err error)
	// WriteMsgUDP writes a message to addr via c if c isn't connected, or
	// to c's remote address if c is connected (in which case addr must be
	// nil). The payload is copied from b and the associated out-of-band
	// data is copied from oob. It returns the number of payload and
	// out-of-band bytes written.
	//
	// The packages golang.org/x/net/ipv4 and golang.org/x/net/ipv6 can be
	// used to manipulate IP-level socket options in oob.
	WriteMsgUDP(b, oob []byte, addr *net.UDPAddr, handler WriteMsgHandler) (err error)
	// WriteMsgUDPAddrPort is like WriteMsgUDP but takes a netip.AddrPort instead of a UDPAddr.
	WriteMsgUDPAddrPort(b, oob []byte, addr netip.AddrPort, handler WriteMsgHandler) (err error)
}

type ReadFromUnixHandler func(n int, addr *net.UnixAddr, err error)
type ReadMsgUnixHandler func(n int, oob []byte, flags int, addr *net.UnixAddr, err error)

type UnixConnection interface {
	Connection
	PacketConnection
	// ReadFromUnix acts like [UnixConn.ReadFrom] but returns a [UnixAddr].
	ReadFromUnix(p []byte, handler ReadFromUnixHandler) (err error)
	// ReadMsgUnix reads a message from c, copying the payload into b and
	// the associated out-of-band data into oob. It returns the number of
	// bytes copied into b, the number of bytes copied into oob, the flags
	// that were set on the message and the source address of the message.
	//
	// Note that if len(b) == 0 and len(oob) > 0, this function will still
	// read (and discard) 1 byte from the connection.
	ReadMsgUnix(p []byte, handler ReadMsgUnixHandler) (err error)
	// WriteToUnix acts like [UnixConn.WriteTo] but takes a [UnixAddr].
	WriteToUnix(p []byte, addr *net.UnixAddr, handler WriteHandler) (err error)
	// WriteMsgUnix writes a message to addr via c, copying the payload
	// from b and the associated out-of-band data from oob. It returns the
	// number of payload and out-of-band bytes written.
	//
	// Note that if len(b) == 0 and len(oob) > 0, this function will still
	// write 1 byte to the connection.
	WriteMsgUnix(b, oob []byte, addr *net.UnixAddr, handler WriteMsgHandler) (err error)
}

type UnixAcceptHandler func(conn UnixConnection, err error)

type UnixListener interface {
	Addr() (addr net.Addr)
	AcceptUnix(handler UnixAcceptHandler)
	Close() (err error)
	polling()
}
