package transport

import (
	"net"
)

type PacketInbound interface {
	Reader() (r InboundReader)
	Received() (n int)
	Addr() (addr net.Addr)
}
