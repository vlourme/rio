package transport

import (
	"net"
)

type MsgInbound interface {
	Reader() (r InboundReader)
	Received() (n int)
	OOB() (r InboundReader)
	OOReceived() (n int)
	Flags() (n int)
	Addr() (addr *net.UDPAddr)
}

type MsgOutbound interface {
	Wrote() (n int)
	OOBWrote() (n int)
	UnexpectedError() (err error)
}
