package security

import (
	"context"
	"errors"
	"fmt"
	"github.com/brickingsoft/rxp/async"
	"net"
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

func (conn *connection) Close() (future async.Future[async.Void]) {
	ctx := conn.Context()
	// Interlock with Conn.Write above.
	var x int32
	for {
		x = conn.activeCall.Load()
		if x&1 != 0 {
			future = async.FailedImmediately[async.Void](ctx, net.ErrClosed)
			return
		}
		if conn.activeCall.CompareAndSwap(x, x|1) {
			break
		}
	}
	if x != 0 {
		// io.Writer and io.Closer should not be used concurrently.
		// If Close is called while a Write is currently in-flight,
		// interpret that as a sign that this Close is really just
		// being used to break the Write and/or clean up resources and
		// avoid sending the alertCloseNotify, which may block
		// waiting on handshakeMutex or the c.out mutex.
		future = conn.Connection.Close()
		return
	}
	promise, promiseErr := async.Make[async.Void](ctx, async.WithWait())
	if promiseErr != nil {
		future = conn.Connection.Close()
		return
	}
	future = promise.Future()

	if conn.handshakeComplete.Load() {
		conn.closeNotify().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
			var alertErr error
			if cause != nil {
				alertErr = fmt.Errorf("tls: failed to send closeNotify alert (but connection was closed anyway): %w", cause)
			}
			conn.Connection.Close().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
				if cause != nil {
					promise.Fail(cause)
					return
				}
				if alertErr != nil {
					promise.Fail(alertErr)
					return
				}
				promise.Succeed(async.Void{})
				return
			})
		})

	}
	return
}
