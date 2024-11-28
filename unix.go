package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/aio"
)

type UnixConnection interface {
	PacketConnection
}

func newUnixConnection(ctx context.Context, fd aio.NetFd) (conn UnixConnection) {
	conn = newPacketConnection(ctx, fd)
	return
}
