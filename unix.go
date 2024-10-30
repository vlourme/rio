package rio

import (
	"github.com/brickingsoft/rio/pkg/async"
	"github.com/brickingsoft/rio/pkg/bytebufferpool"
	"net"
)

// tcp: unix,unixpacket
// udp: unixgram

type UnixInbound interface {
	Buffer() (buf bytebufferpool.Buffer)
	Bytes() (n int)
	Addr() (addr *net.UnixAddr)
}

type UnixMsgInbound interface {
	Buffer() (buf bytebufferpool.Buffer)
	Bytes() (n int)
	OOBBytes() (n int)
	Flags() (n int)
	Addr() (addr *net.UnixAddr)
}

type UnixConnection interface {
	Connection
	PacketConnection
	// ReadFromUnix acts like [UnixConn.ReadFrom] but returns a [UnixAddr].
	ReadFromUnix() (future async.Future[UnixInbound])
	// ReadMsgUnix reads a message from c, copying the payload into b and
	// the associated out-of-band data into oob. It returns the number of
	// bytes copied into b, the number of bytes copied into oob, the flags
	// that were set on the message and the source address of the message.
	//
	// Note that if len(b) == 0 and len(oob) > 0, this function will still
	// read (and discard) 1 byte from the connection.
	ReadMsgUnix() (future async.Future[UnixMsgInbound])
	// WriteToUnix acts like [UnixConn.WriteTo] but takes a [UnixAddr].
	WriteToUnix(b []byte, addr *net.UnixAddr) (future async.Future[Outbound])
	// WriteMsgUnix writes a message to addr via c, copying the payload
	// from b and the associated out-of-band data from oob. It returns the
	// number of payload and out-of-band bytes written.
	//
	// Note that if len(b) == 0 and len(oob) > 0, this function will still
	// write 1 byte to the connection.
	WriteMsgUnix(b, oob []byte, addr *net.UnixAddr) (future async.Future[MsgOutbound])
}
