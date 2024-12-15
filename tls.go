package rio

import (
	"context"
	"crypto/tls"
	"github.com/brickingsoft/rio/pkg/aio"
	"github.com/brickingsoft/rio/pkg/security"
)

type TLSConnectionBuilder func(ctx context.Context, fd aio.NetFd, config *tls.Config) (sc *security.TLSConnection, err error)
