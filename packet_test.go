package rio_test

import (
	"context"
	"github.com/brickingsoft/rio"
	"github.com/brickingsoft/rio/transport"
	"sync"
	"testing"
)

func TestListenPacket(t *testing.T) {
	ctx := context.Background()
	srv, lnErr := rio.ListenPacket(ctx, "udp", ":9000")
	if lnErr != nil {
		t.Error(lnErr)
		return
	}
	defer srv.Close()

	wg := new(sync.WaitGroup)
	wg.Add(1)
	srv.ReadFrom().OnComplete(func(ctx context.Context, entry transport.PacketInbound, cause error) {
		defer wg.Done()
		if cause != nil {
			t.Error("srv read from:", cause)
			return
		}
		p, _ := entry.Reader().Next(entry.Received())
		t.Log("srv read from:", entry.Addr(), entry.Received(), string(p))
		wg.Add(1)
		srv.WriteTo(p[0:entry.Received()], entry.Addr()).OnComplete(func(ctx context.Context, entry transport.Outbound, cause error) {
			defer wg.Done()
			if cause != nil {
				t.Error("srv write to:", cause)
				return
			}
			t.Log("srv write to:", entry.Wrote(), entry.UnexpectedError())
		})
	})

	wg.Add(1)
	rio.Dial(ctx, "udp", "127.0.0.1:9000").OnComplete(func(ctx context.Context, conn rio.Connection, cause error) {
		defer wg.Done()
		if cause != nil {
			t.Error("cli read dial err:", cause)
			return
		}
		wg.Add(1)
		conn.Write([]byte("hello world")).OnComplete(func(ctx context.Context, entry transport.Outbound, cause error) {
			defer wg.Done()
			if cause != nil {
				t.Error("cli write err:", cause)
				return
			}
			t.Log("cli write:", entry.Wrote(), entry.UnexpectedError())
			wg.Add(1)
			conn.Read().OnComplete(func(ctx context.Context, entry transport.Inbound, cause error) {
				defer wg.Done()
				if cause != nil {
					t.Error("cli read err:", cause)
					return
				}
				t.Log("cli read:", string(entry.Reader().Peek(entry.Received())))
			})
		})
	})

	wg.Wait()
}
