package transport

import (
	"net"
)

type PacketInbound interface {
	Reader() (r InboundReader)
	Received() (n int)
	Addr() (addr net.Addr)
}

type MsgInbound interface {
	Reader() (r InboundReader)
	Received() (n int)
	OOB() (r InboundReader)
	OOReceived() (n int)
	Flags() (n int)
	Addr() (addr net.Addr)
}

type MsgOutbound interface {
	Wrote() (n int)
	OOBWrote() (n int)
	UnexpectedError() (err error)
}
