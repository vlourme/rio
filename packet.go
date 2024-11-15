package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/sockets"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"net"
)

// ListenPacket
// "udp", "udp4", "udp6", "unixgram", "ip"
func ListenPacket(ctx context.Context, network string, addr string, options ...Option) (conn PacketConnection, err error) {
	opts := Options{}
	for _, o := range options {
		err = o(&opts)
		if err != nil {
			return
		}
	}
	socketOpts := sockets.Options{
		MultipathTCP:            opts.MultipathTCP,
		DialPacketConnLocalAddr: opts.DialPacketConnLocalAddr,
	}
	inner, innerErr := sockets.ListenPacket(network, addr, socketOpts)
	if innerErr != nil {
		err = innerErr
		return
	}

	conn = &packetConnection{
		connection: *newConnection(ctx, inner),
	}
	return
}

type PacketConnection interface {
	Connection
	ReadFrom() (future async.Future[transport.PacketInbound])
	WriteTo(p []byte, addr net.Addr) (future async.Future[transport.Outbound])
	SetReadMsgUDPOOBBufferSize(size int)
	ReadMsg() (future async.Future[transport.MsgInbound])
	WriteMsg(p []byte, oob []byte, addr net.Addr) (future async.Future[transport.MsgOutbound])
}

type packetConnection struct {
	connection
}

func (conn *packetConnection) ReadFrom() (future async.Future[transport.PacketInbound]) {
	//TODO implement me
	panic("implement me")
}

func (conn *packetConnection) WriteTo(p []byte, addr net.Addr) (future async.Future[transport.Outbound]) {
	//TODO implement me
	panic("implement me")
}

func (conn *packetConnection) SetReadMsgUDPOOBBufferSize(size int) {
	//TODO implement me
	panic("implement me")
}

func (conn *packetConnection) ReadMsg() (future async.Future[transport.MsgInbound]) {
	//TODO implement me
	panic("implement me")
}

func (conn *packetConnection) WriteMsg(p []byte, oob []byte, addr net.Addr) (future async.Future[transport.MsgOutbound]) {
	//TODO implement me
	panic("implement me")
}
