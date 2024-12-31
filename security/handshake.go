package security

import (
	"github.com/brickingsoft/rxp/async"
)

type Handshake func() (future async.Future[async.Void])
