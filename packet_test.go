package rio_test

import (
	"context"
	"github.com/brickingsoft/rio"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"testing"
)

func TestListenPacket(t *testing.T) {
	ctx := context.Background()
	ctx = withWG(ctx)

	srv, lnErr := rio.ListenPacket(ctx, "udp", ":9000")
	if lnErr != nil {
		t.Error(lnErr)
		return
	}
	defer func() {
		srv.Close().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
			if cause != nil {
				t.Error(cause)
			}
		})
	}()

	wgAdd(ctx)
	srv.ReadFrom().OnComplete(func(ctx context.Context, entry transport.PacketInbound, cause error) {
		defer wgDone(ctx)
		if cause != nil {
			t.Error("srv read from:", cause)
			return
		}
		p, _ := entry.Reader().Next(entry.Received())
		t.Log("srv read from:", entry.Addr(), entry.Received(), string(p))
		wgAdd(ctx)
		srv.WriteTo(p[0:entry.Received()], entry.Addr()).OnComplete(func(ctx context.Context, entry transport.Outbound, cause error) {
			defer wgDone(ctx)
			if cause != nil {
				t.Error("srv write to:", cause)
				return
			}
			t.Log("srv write to:", entry.Wrote(), entry.UnexpectedError())
		})
	})

	wgAdd(ctx)
	rio.Dial(ctx, "udp", "127.0.0.1:9000").OnComplete(func(ctx context.Context, conn rio.Connection, cause error) {
		defer wgDone(ctx)
		if cause != nil {
			t.Error("cli read dial err:", cause)
			return
		}
		wgAdd(ctx)
		conn.Write([]byte("hello world")).OnComplete(func(ctx context.Context, entry transport.Outbound, cause error) {
			defer wgDone(ctx)
			if cause != nil {
				t.Error("cli write err:", cause)
				return
			}
			t.Log("cli write:", entry.Wrote(), entry.UnexpectedError())
			wgAdd(ctx)
			conn.Read().OnComplete(func(ctx context.Context, entry transport.Inbound, cause error) {
				defer wgDone(ctx)
				if cause != nil {
					t.Error("cli read err:", cause)
					return
				}
				t.Log("cli read:", string(entry.Reader().Peek(entry.Received())))
				conn.Close().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
					if cause != nil {
						t.Error("cli close:", cause)
					}
				})
			})
		})
	})

	wgWait(ctx)
}
