package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/aio"
)

type IPConnection interface {
	PacketConnection
}

func newIPConnection(ctx context.Context, fd aio.NetFd) (conn IPConnection) {
	conn = newPacketConnection(ctx, fd)
	return
}
