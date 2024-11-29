package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/aio"
	"github.com/brickingsoft/rxp/async"
	"net"
)

var (
	ErrClosed           = errors.New("rio: closed")
	ErrBusy             = errors.New("rio: system busy")
	ErrEmptyBytes       = errors.New("rio: empty bytes")
	ErrNetworkUnmatched = errors.New("rio: network is not matched")
	ErrNilAddr          = errors.New("rio: addr is nil")
	ErrAllocate         = errors.New("rio: allocate bytes failed")
	ErrAllocateWrote    = errors.New("rio: allocate wrote failed")
)

func IsClosed(err error) bool {
	var opErr *net.OpError
	isOpErr := errors.As(err, &opErr)
	if isOpErr {
		err = opErr.Err
	}
	ok := errors.Is(err, async.EOF) || errors.Is(err, async.UnexpectedEOF) ||
		errors.Is(err, ErrClosed) ||
		errors.Is(err, context.Canceled) || errors.Is(err, async.UnexpectedContextFailed) ||
		errors.Is(err, async.ExecutorsClosed)
	return ok
}

func IsBusy(err error) bool {
	var opErr *net.OpError
	isOpErr := errors.As(err, &opErr)
	if isOpErr {
		err = opErr.Err
	}
	return errors.Is(err, ErrBusy)
}

func IsErrEmptyBytes(err error) bool {
	var opErr *net.OpError
	isOpErr := errors.As(err, &opErr)
	if isOpErr {
		err = opErr.Err
	}
	return errors.Is(err, ErrEmptyBytes)
}

func IsErrNetworkUnmatched(err error) bool {
	var opErr *net.OpError
	isOpErr := errors.As(err, &opErr)
	if isOpErr {
		err = opErr.Err
	}
	return errors.Is(err, ErrNetworkUnmatched)
}

func IsErrAllocate(err error) bool {
	var opErr *net.OpError
	isOpErr := errors.As(err, &opErr)
	if isOpErr {
		err = opErr.Err
	}
	return errors.Is(err, ErrAllocate)
}

func IsErrAllocateWrote(err error) bool {
	var opErr *net.OpError
	isOpErr := errors.As(err, &opErr)
	if isOpErr {
		err = opErr.Err
	}
	return errors.Is(err, ErrAllocateWrote)
}

const (
	opDial   = "dial"
	opListen = "listen"
	opAccept = "accept"
	opRead   = "read"
	opWrite  = "write"
	opClose  = "close"
	opSet    = "set"
)

func newOpErr(op string, fd aio.NetFd, err error) *net.OpError {
	return &net.OpError{
		Op:     op,
		Net:    fd.Network(),
		Source: fd.LocalAddr(),
		Addr:   fd.RemoteAddr(),
		Err:    err,
	}
}
