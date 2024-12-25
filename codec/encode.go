package codec

import (
	"context"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
)

type Encoder[T any] interface {
	Encode(param T) (b []byte, err error)
}

func Encode[T any](ctx context.Context, encoder Encoder[T], writer transport.Writer, data T) (future async.Future[int]) {
	p, encodeErr := encoder.Encode(data)
	if encodeErr != nil {
		future = async.FailedImmediately[int](ctx, encodeErr)
		return
	}
	future = writer.Write(p)
	return
}
