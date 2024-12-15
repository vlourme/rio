package security

import (
	"context"
	"crypto/tls"
	"github.com/brickingsoft/rio/pkg/aio"
)

func Client(ctx context.Context, fd aio.NetFd, config *tls.Config) (sc *TLSConnection, err error) {
	return
}
