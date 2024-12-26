package codec

import (
	"context"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
)

// Decoder
// 解析器。
// 泛型 T 是解析的结果，建议在结果中自行定义协议解析的错误，因为 Decode 的错误会停止解析。
// 或者在实现体中自行处理协议解析的错误，来决定是否返回 err 来停止解析。
type Decoder[T any] interface {
	// Decode
	// 解析 transport.Inbound。
	// 返回 ok(是否解析到，即message是否为空)，message(消息)，err(错误，并停止解析)
	Decode(inbound transport.Inbound) (ok bool, message T, err error)
}

// Decode
// 流式解析
// 默认创建一个流式且无限等待的 async.Promise。
func Decode[T any](ctx context.Context, reader transport.Reader, decoder Decoder[T], options ...async.Option) (future async.Future[T]) {
	// 默认开启 流 和 强等
	options = append(options, async.WithStream(), async.WithWait())
	promise, promiseErr := async.Make[T](ctx, options...)
	if promiseErr != nil {
		future = async.FailedImmediately[T](ctx, promiseErr)
		return
	}
	decode[T](reader, decoder, true, promise)
	future = promise.Future()
	return
}

// DecodeOnce
// 单次解析
func DecodeOnce[T any](ctx context.Context, reader transport.Reader, decoder Decoder[T], options ...async.Option) (future async.Future[T]) {
	promise, promiseErr := async.Make[T](ctx, options...)
	if promiseErr != nil {
		future = async.FailedImmediately[T](ctx, promiseErr)
		return
	}
	decode[T](reader, decoder, false, promise)
	future = promise.Future()
	return
}

func decode[T any](reader transport.Reader, decoder Decoder[T], stream bool, promise async.Promise[T]) {
	reader.Read().OnComplete(func(ctx context.Context, result transport.Inbound, err error) {
		if err != nil {
			promise.Fail(err)
			if stream {
				promise.Cancel()
			}
			return
		}
		ok, message, decodeErr := decoder.Decode(result)
		if decodeErr != nil {
			// 解析错误并停止解析
			promise.Fail(decodeErr)
			return
		}
		if ok {
			// 解析到
			promise.Succeed(message)
			if stream {
				// 流式则循环
				decode[T](reader, decoder, stream, promise)
			}
		} else {
			// 未解析到则继续
			decode[T](reader, decoder, stream, promise)
		}
	})
	return
}
