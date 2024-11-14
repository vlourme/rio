package rio

import (
	"context"
	"errors"
)

var (
	ErrClosed            = errors.New("rio: closed")
	ErrBusy              = errors.New("rio: system busy")
	ErrEmptyPacket       = errors.New("rio: empty packet")
	ErrNetworkDisMatched = errors.New("rio: network is not matched")
)

func IsClosed(err error) bool {
	return errors.Is(err, ErrClosed) || errors.Is(err, context.Canceled)
}
