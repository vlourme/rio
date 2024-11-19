package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rxp/async"
)

var (
	ErrClosed           = errors.New("rio: closed")
	ErrBusy             = errors.New("rio: system busy")
	ErrEmptyPacket      = errors.New("rio: empty packet")
	ErrNetworkUnmatched = errors.New("rio: network is not matched")
	ErrNilAddr          = errors.New("rio: addr is nil")
)

func IsClosed(err error) bool {
	return errors.Is(err, ErrClosed) || errors.Is(err, context.Canceled) || errors.Is(err, async.EOF)
}
