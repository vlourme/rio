package codec

import (
	"context"
	"github.com/brickingsoft/rio/pkg/async"
	"github.com/brickingsoft/rio/transport"
)

type FutureReader interface {
	Read() (future async.Future[transport.Inbound])
}

type Decoder[T any] interface {
	Decode(inbound transport.Inbound) (message T, next bool, err error)
}

func Decode[T any](ctx context.Context, reader FutureReader, decoder Decoder[T]) (future async.Future[T]) {
	promise, promiseErr := async.MustPromise[T](ctx)
	if promiseErr != nil {
		future = async.FailedImmediately[T](ctx, promiseErr)
		return
	}
	decode[T](reader, decoder, promise)
	future = promise.Future()
	return
}

func InfiniteDecode[T any](ctx context.Context, reader FutureReader, decoder Decoder[T], infiniteBuf int) (future async.Future[T]) {
	promise, promiseErr := async.MustInfinitePromise[T](ctx, infiniteBuf)
	if promiseErr != nil {
		future = async.FailedImmediately[T](ctx, promiseErr)
		return
	}
	decode[T](reader, decoder, promise)
	future = promise.Future()
	return
}

func decode[T any](reader FutureReader, decoder Decoder[T], promise async.Promise[T]) {
	reader.Read().OnComplete(func(ctx context.Context, result transport.Inbound, err error) {
		if err != nil {
			promise.Fail(err)
			return
		}
		message, next, decodeErr := decoder.Decode(result)
		if decodeErr != nil {
			promise.Fail(decodeErr)
			return
		}
		promise.Succeed(message)
		if next {
			decode[T](reader, decoder, promise)
		}
	})
	return
}
