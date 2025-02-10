package rio_test

import (
	"context"
	"github.com/brickingsoft/rio"
	"github.com/brickingsoft/rio/transport"
	"sync"
	"testing"
)

func TestListenPacket(t *testing.T) {
	rio.Startup()
	defer rio.Shutdown()

	ctx := rio.Background()

	srv, lnErr := rio.ListenPacket("udp", ":9000")
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
		p, _ := entry.Next(entry.Len())
		addr := entry.Addr()
		t.Log("srv read from:", addr, string(p))
		srv.WriteTo(p, addr).OnComplete(func(ctx context.Context, entry int, cause error) {
			defer lwg.Done()
			if cause != nil {
				t.Error("srv write to:", cause)
				return
			}
			t.Log("srv write to:", entry)
		})
	})

	cwg := new(sync.WaitGroup)
	cwg.Add(1)
	rio.Dial(ctx, "udp", "127.0.0.1:9000").OnComplete(func(ctx context.Context, conn transport.Connection, cause error) {
		if cause != nil {
			t.Error("cli read dial err:", cause)
			cwg.Done()
			return
		}
		conn.Write([]byte("hello world")).OnComplete(func(ctx context.Context, entry int, cause error) {
			if cause != nil {
				t.Error("cli write err:", cause)
				cwg.Done()
				return
			}
			t.Log("cli write:", entry)
			conn.Read().OnComplete(func(ctx context.Context, entry transport.Inbound, cause error) {
				if cause != nil {
					t.Error("cli read err:", cause)
					cwg.Done()
					return
				}
				b, _ := entry.Next(entry.Len())
				t.Log("cli read:", string(b))
				_ = conn.Close()
				cwg.Done()
			})
		})
	})

	cwg.Wait()

	_ = srv.Close()
	lwg.Wait()
}

func TestListenPacketMsg(t *testing.T) {
	rio.Startup()
	defer rio.Shutdown()

	ctx := rio.Background()

	srv, lnErr := rio.ListenPacket("udp", ":9000")
	if lnErr != nil {
		t.Error(lnErr)
		return
	}

	lwg := new(sync.WaitGroup)
	lwg.Add(1)
	srv.ReadMsg().OnComplete(func(ctx context.Context, entry transport.PacketMsgInbound, cause error) {
		if cause != nil {
			t.Error("srv read from:", cause)
			lwg.Done()
			return
		}
		b, _ := entry.Next(entry.Len())
		oob := entry.OOB()
		t.Log("srv read bytes from:", entry.Addr(), string(b))
		t.Log("srv read oob from:", entry.Addr(), string(oob))

		srv.WriteMsg(b, nil, entry.Addr()).OnComplete(func(ctx context.Context, entry transport.PacketMsgOutbound, cause error) {
			defer lwg.Done()
			if cause != nil {
				t.Error("srv write to:", cause)
				return
			}
			t.Log("srv write to:", entry.N, entry.OOBN)
		})
	})

	cwg := new(sync.WaitGroup)
	cwg.Add(1)
	rio.Dial(ctx, "udp", "127.0.0.1:9000").OnComplete(func(ctx context.Context, conn transport.Connection, cause error) {
		if cause != nil {
			t.Error("cli read dial err:", cause)
			cwg.Done()
			return
		}
		pack := conn.(transport.PacketConnection)
		pack.Write([]byte("hello world")).OnComplete(func(ctx context.Context, n int, cause error) {
			if cause != nil {
				t.Error("cli write err:", cause)
				cwg.Done()
				return
			}
			t.Log("cli write:", n)
			pack.ReadMsg().OnComplete(func(ctx context.Context, entry transport.PacketMsgInbound, cause error) {
				if cause != nil {
					t.Error("cli read err:", cause)
					cwg.Done()
					return
				}
				b, _ := entry.Next(entry.Len())
				oob := entry.OOB()
				t.Log("cli read:", string(b), string(oob), entry.Addr(), entry.Flags())
				_ = conn.Close()
				cwg.Done()
			})
		})
	})

	cwg.Wait()

	_ = srv.Close()
	lwg.Wait()
}
