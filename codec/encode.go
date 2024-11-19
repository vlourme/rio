package codec

import (
	"context"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
)

type FutureWriter interface {
	Write(p []byte) (future async.Future[transport.Outbound])
}

type Encoder[T any] interface {
	Encode(param T) (p []byte, err error)
}

func Encode[T any](ctx context.Context, encoder Encoder[T], writer FutureWriter, data T) (future async.Future[transport.Outbound]) {
	p, encodeErr := encoder.Encode(data)
	if encodeErr != nil {
		future = async.FailedImmediately[transport.Outbound](ctx, encodeErr)
		return
	}
	future = writer.Write(p)
	return
}
