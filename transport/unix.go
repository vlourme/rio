package transport

import "net"

type UnixMsgInbound interface {
	Reader() (r InboundReader)
	Received() (n int)
	OOB() (r InboundReader)
	OOReceived() (n int)
	Flags() (n int)
	Addr() (addr *net.UnixAddr)
}
