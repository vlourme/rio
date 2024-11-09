package transport

import "net"

type UnixInbound interface {
	Reader() (r InboundReader)
	Received() (n int)
	Addr() (addr *net.UnixAddr)
}

type UnixMsgInbound interface {
	Reader() (r InboundReader)
	Received() (n int)
	OOBBytes() (n int)
	Flags() (n int)
	Addr() (addr *net.UnixAddr)
}
