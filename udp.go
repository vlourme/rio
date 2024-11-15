package rio

import (
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"net"
)

type UPDConnection interface {
	PacketConnection
	ReadMsgUDP() (future async.Future[transport.MsgInbound])
	WriteMsgUDP(p []byte, oob []byte, addr *net.UDPAddr) (future async.Future[transport.MsgOutbound])
}
