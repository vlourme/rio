package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/sockets"
)

type UnixConnection interface {
	PacketConnection
}

func newUnixConnection(ctx context.Context, inner sockets.Connection) (conn UnixConnection) {
	conn = &packetConnection{
		connection: *newConnection(ctx, inner),
	}
	return
}
