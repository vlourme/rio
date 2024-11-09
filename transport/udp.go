package transport

import (
	"net"
	"net/netip"
)

type UPDInbound interface {
	Reader() (r InboundReader)
	Received() (n int)
	Addr() (addr *net.UDPAddr)
}

type UPDAddrPortInbound interface {
	Reader() (r InboundReader)
	Received() (n int)
	Addr() (addr netip.AddrPort)
}

type UPDMsgInbound interface {
	Reader() (r InboundReader)
	Received() (n int)
	OOBBytes() (n int)
	Flags() (n int)
	Addr() (addr *net.UDPAddr)
}
type UPDMsgAddrPortInbound interface {
	Reader() (r InboundReader)
	Received() (n int)
	OOBBytes() (n int)
	Flags() (n int)
	Addr() (addr netip.AddrPort)
}

type MsgOutbound interface {
	Wrote() (n int)
	OOBBytes() (n int)
	UnexpectedError() (err error)
}
