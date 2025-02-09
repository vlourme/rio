package security

import (
	"context"
	"errors"
	"github.com/brickingsoft/rxp/async"
	"time"
)

func (conn *connection) closeNotify() (future async.Future[async.Void]) {
	ctx := conn.Context()

	if !conn.closeNotifySent {
		promise, promiseErr := async.Make[async.Void](ctx, async.WithWait())
		if promiseErr != nil {
			future = async.FailedImmediately[async.Void](ctx, promiseErr)
			return
		}
		future = promise.Future()

		// Set a Write Deadline to prevent possibly blocking forever.
		conn.SetWriteTimeout(time.Second * 5)
		conn.sendAlertLocked(alertCloseNotify).OnComplete(func(ctx context.Context, entry async.Void, cause error) {
			// Any subsequent writes will fail.
			// todo use
			conn.SetWriteTimeout(0)
			if cause != nil {
				conn.closeNotifyErr = cause
				conn.closeNotifySent = true
				promise.Fail(cause)
				return
			}
			promise.Succeed(async.Void{})
			return
		})
		return
	}
	if conn.closeNotifyErr != nil {
		future = async.FailedImmediately[async.Void](ctx, conn.closeNotifyErr)
	} else {
		future = async.SucceedImmediately[async.Void](ctx, async.Void{})
	}
	return
}

var errEarlyCloseWrite = errors.New("rio: CloseWrite called before handshake complete")

func (conn *connection) CloseWrite() (future async.Future[async.Void]) {
	ctx := conn.Context()
	if !conn.handshakeComplete.Load() {
		future = async.FailedImmediately[async.Void](ctx, errEarlyCloseWrite)
		return
	}
	if conn.closeNotifySent {
		if conn.closeNotifyErr != nil {
			future = async.FailedImmediately[async.Void](ctx, conn.closeNotifyErr)
			return
		}
		future = async.SucceedImmediately[async.Void](ctx, async.Void{})
		return
	}

	promise, promiseErr := async.Make[async.Void](ctx, async.WithWait())
	if promiseErr != nil {
		future = async.FailedImmediately[async.Void](ctx, promiseErr)
		return
	}
	future = promise.Future()

	conn.closeNotify().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
		conn.closeNotifySent = true
		if cause != nil {
			conn.closeNotifyErr = cause
			promise.Fail(cause)
		} else {
			promise.Succeed(async.Void{})
		}
		return
	})
	return
}

func (conn *connection) Close() (err error) {

	return
}
