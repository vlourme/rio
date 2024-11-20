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
	ErrAllocate         = errors.New("rio: allocate bytes failed")
)

func IsClosed(err error) bool {
	ok := errors.Is(err, async.EOF) || errors.Is(err, async.UnexpectedEOF) ||
		errors.Is(err, ErrClosed) ||
		errors.Is(err, context.Canceled) || errors.Is(err, async.UnexpectedContextFailed) ||
		errors.Is(err, async.ExecutorsClosed)
	return ok
}
