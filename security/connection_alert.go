package security

import (
	"context"
	"github.com/brickingsoft/rxp/async"
	"net"
)

func (conn *connection) sendAlertLocked(err alert) (future async.Future[async.Void]) {
	ctx := conn.Context()
	promise, promiseErr := async.Make[async.Void](ctx, async.WithWait())
	if promiseErr != nil {
		future = async.FailedImmediately[async.Void](ctx, promiseErr)
		return
	}
	future = promise.Future()

	switch err {
	case alertNoRenegotiation, alertCloseNotify:
		conn.tmp[0] = alertLevelWarning
	default:
		conn.tmp[0] = alertLevelError
	}
	conn.tmp[1] = byte(err)

	conn.writeRecordLocked(recordTypeAlert, conn.tmp[0:2]).OnComplete(func(ctx context.Context, entry int, cause error) {
		if err == alertCloseNotify {
			promise.Fail(cause)
			return
		}
		cause = conn.out.setErrorLocked(&net.OpError{Op: "local error", Err: err})
		promise.Fail(cause)
		return
	})

	return
}

// sendAlert sends a TLS alert message.
func (conn *connection) sendAlert(err alert) async.Future[async.Void] {
	return conn.sendAlertLocked(err)
}
