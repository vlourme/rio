package rio_test

import (
	"context"
	"github.com/brickingsoft/rio"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"sync"
	"testing"
)

func TestListenPacket(t *testing.T) {
	_ = rio.Startup()
	defer func() {
		_ = rio.ShutdownGracefully()
	}()

	ctx := context.Background()

	srv, lnErr := rio.ListenPacket(ctx, "udp", ":9000")
	if lnErr != nil {
		t.Error(lnErr)
		return
	}

	lwg := new(sync.WaitGroup)
	lwg.Add(1)
	srv.ReadFrom().OnComplete(func(ctx context.Context, entry transport.PacketInbound, cause error) {
		if cause != nil {
			t.Error("srv read from:", cause)
			lwg.Done()
			return
		}
		p, _ := entry.Reader().Next(entry.Received())
		t.Log("srv read from:", entry.Addr(), entry.Received(), string(p))
		srv.WriteTo(p[0:entry.Received()], entry.Addr()).OnComplete(func(ctx context.Context, entry transport.Outbound, cause error) {
			defer lwg.Done()
			if cause != nil {
				t.Error("srv write to:", cause)
				return
			}
			t.Log("srv write to:", entry.Wrote(), entry.UnexpectedError())
		})
	})

	cwg := new(sync.WaitGroup)
	cwg.Add(1)
	rio.Dial(ctx, "udp", "127.0.0.1:9000").OnComplete(func(ctx context.Context, conn rio.Connection, cause error) {
		if cause != nil {
			t.Error("cli read dial err:", cause)
			cwg.Done()
			return
		}
		conn.Write([]byte("hello world")).OnComplete(func(ctx context.Context, entry transport.Outbound, cause error) {
			if cause != nil {
				t.Error("cli write err:", cause)
				cwg.Done()
				return
			}
			t.Log("cli write:", entry.Wrote(), entry.UnexpectedError())
			conn.Read().OnComplete(func(ctx context.Context, entry transport.Inbound, cause error) {
				if cause != nil {
					t.Error("cli read err:", cause)
					cwg.Done()
					return
				}
				t.Log("cli read:", string(entry.Reader().Peek(entry.Received())))
				conn.Close().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
					if cause != nil {
						t.Error("cli close:", cause)
					}
					cwg.Done()
				})
			})
		})
	})

	cwg.Wait()

	lwg.Add(1)
	srv.Close().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
		t.Log("ln close:", cause)
		lwg.Done()
	})
	lwg.Wait()
}
