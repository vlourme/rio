package rio

import (
	"github.com/brickingsoft/rio/async"
	"github.com/brickingsoft/rio/transport"
	"net"
)

type UPDConnection interface {
	PacketConnection
	SetReadMsgUDPOOBBufferSize(size int)
	ReadMsgUDP() (future async.Future[transport.MsgInbound])
	WriteMsgUDP(p []byte, oob []byte, addr *net.UDPAddr) (future async.Future[transport.MsgOutbound])
}
