package rio

import (
	"github.com/brickingsoft/rio/async"
	"github.com/brickingsoft/rio/transport"
	"net"
	"net/netip"
)

type UPDConnection interface {
	PacketConnection
	SetReadMsgUDPOOBBufferSize(size int)
	// ReadFromUDP acts like ReadFrom but returns a UDPAddr.
	ReadFromUDP() (future async.Future[transport.UPDInbound])
	// ReadFromUDPAddrPort acts like ReadFrom but returns a netip.AddrPort.
	//
	// If c is bound to an unspecified address, the returned
	// netip.AddrPort's address might be an IPv4-mapped IPv6 address.
	// Use netip.Addr.Unmap to get the address without the IPv6 prefix.
	ReadFromUDPAddrPort() (future async.Future[transport.UPDAddrPortInbound])
	// ReadMsgUDP reads a message from c, copying the payload into b and
	// the associated out-of-band data into oob. It returns the number of
	// bytes copied into b, the number of bytes copied into oob, the flags
	// that were set on the message and the source address of the message.
	//
	// The packages golang.org/x/net/ipv4 and golang.org/x/net/ipv6 can be
	// used to manipulate IP-level socket options in oob.
	ReadMsgUDP() (future async.Future[transport.UPDMsgInbound])
	// ReadMsgUDPAddrPort is like ReadMsgUDP but returns an netip.AddrPort instead of a UDPAddr.
	ReadMsgUDPAddrPort(b, oob []byte) (future async.Future[transport.UPDMsgAddrPortInbound])
	// WriteToUDP acts like WriteTo but takes a UDPAddr.
	WriteToUDP(b []byte, addr *net.UDPAddr) (future async.Future[transport.Outbound])
	// WriteToUDPAddrPort acts like WriteTo but takes a netip.AddrPort.
	WriteToUDPAddrPort(b []byte, addr netip.AddrPort) (future async.Future[transport.Outbound])
	// WriteMsgUDP writes a message to addr via c if c isn't connected, or
	// to c's remote address if c is connected (in which case addr must be
	// nil). The payload is copied from b and the associated out-of-band
	// data is copied from oob. It returns the number of payload and
	// out-of-band bytes written.
	//
	// The packages golang.org/x/net/ipv4 and golang.org/x/net/ipv6 can be
	// used to manipulate IP-level socket options in oob.
	WriteMsgUDP(b, oob []byte, addr *net.UDPAddr) (future async.Future[transport.MsgOutbound])
	// WriteMsgUDPAddrPort is like WriteMsgUDP but takes a netip.AddrPort instead of a UDPAddr.
	WriteMsgUDPAddrPort(b, oob []byte, addr netip.AddrPort) (future async.Future[transport.MsgOutbound])
}
