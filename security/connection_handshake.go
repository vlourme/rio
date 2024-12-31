package security

import (
	"context"
	"github.com/brickingsoft/rxp/async"
)

func (conn *connection) Handshake() (future async.Future[async.Void]) {
	ctx := conn.Context()
	if conn.handshakeComplete.Load() {
		if conn.handshakeErr != nil {
			future = async.FailedImmediately[async.Void](ctx, conn.handshakeErr)
		} else {
			future = async.SucceedImmediately[async.Void](ctx, async.Void{})
		}
		return
	}

	future = conn.handshakeBarrier.Do(ctx, conn.handshakeBarrierKey, func(promise async.Promise[async.Void]) {
		conn.handshakeFn().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
			conn.handshakeComplete.Store(true)
			if cause != nil {
				conn.handshakeErr = cause
				promise.Fail(cause)
				return
			}
			// todo handle handshake result
			promise.Succeed(async.Void{})
			return
		})
	}, async.WithWait())

	return
}
