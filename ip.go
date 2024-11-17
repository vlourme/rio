package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/sockets"
)

type IPConnection interface {
	PacketConnection
}

func newIPConnection(ctx context.Context, inner sockets.Connection) (conn IPConnection) {
	conn = newPacketConnection(ctx, inner.(sockets.PacketConnection))
	return
}
