package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/sockets"
)

type UPDConnection interface {
	PacketConnection
}

func newUDPConnection(ctx context.Context, inner sockets.PacketConnection) (conn UPDConnection) {
	conn = &packetConnection{
		connection: *newConnection(ctx, inner),
	}
	return
}
