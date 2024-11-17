package rio_test

import (
	"context"
	"github.com/brickingsoft/rio"
	"github.com/brickingsoft/rio/pkg/rate/timeslimiter"
	"github.com/brickingsoft/rio/transport"
	"net"
	"sync"
	"testing"
)

func TestListenTCP(t *testing.T) {
	ctx := context.Background()
	ln, lnErr := rio.Listen(
		ctx,
		"tcp", "127.0.0.1:9000",
		rio.WithParallelAcceptors(1),
		rio.WithMaxConnections(10),
	)
	if lnErr != nil {
		t.Error(lnErr)
		return
	}
	ln.Accept().OnComplete(func(ctx context.Context, conn rio.Connection, err error) {
		if err != nil {
			t.Log("accepted:", timeslimiter.Tokens(ctx), err, ctx.Err())
			return
		}
		var addr net.Addr
		if conn != nil {
			addr = conn.RemoteAddr()
		}
		t.Log("accepted:", timeslimiter.Tokens(ctx), addr, err, ctx.Err())
		if conn != nil {
			err = conn.Close()
			if err != nil {
				t.Error("srv close conn err:", err)
			}
		}
	})
	for i := 0; i < 10; i++ {
		conn, dialErr := net.Dial("tcp", ":9000")
		if dialErr != nil {
			t.Error(dialErr)
			return
		}
		t.Log("dialed:", i+1, conn.LocalAddr())
		err := conn.Close()
		if err != nil {
			t.Error("cli close conn err:", err)
		}
	}
	closeErr := ln.Close()
	if closeErr != nil {
		t.Error(closeErr)
	}
}

func withWG(ctx context.Context) context.Context {
	return context.WithValue(ctx, "wg", new(sync.WaitGroup))
}

func wgAdd(ctx context.Context) {
	wg := ctx.Value("wg").(*sync.WaitGroup)
	wg.Add(1)
}

func wgDone(ctx context.Context) {
	wg := ctx.Value("wg").(*sync.WaitGroup)
	wg.Done()
}

func wgWait(ctx context.Context) {
	wg := ctx.Value("wg").(*sync.WaitGroup)
	wg.Wait()
}

func TestTCP(t *testing.T) {
	ctx := context.Background()

	ln, lnErr := rio.Listen(ctx, "tcp", ":9000", rio.WithParallelAcceptors(1))
	if lnErr != nil {
		t.Error(lnErr)
		return
	}
	defer ln.Close()

	ln.Accept().OnComplete(func(ctx context.Context, conn rio.Connection, err error) {
		if err != nil {
			if rio.IsClosed(err) {
				t.Log("closed")
			} else {
				t.Error("srv accept:", rio.IsClosed(err), err)
			}
			return
		}
		t.Log("srv accept:", conn.RemoteAddr(), err)
		conn.Read().OnComplete(func(ctx context.Context, in transport.Inbound, err error) {
			if err != nil {
				t.Error("srv read:", err)
				_ = conn.Close()
				return
			}
			n := in.Received()
			p, _ := in.Reader().Next(n)
			t.Log("srv read:", n, string(p))
			conn.Write(p).OnComplete(func(ctx context.Context, out transport.Outbound, err error) {
				if err != nil {
					t.Error("srv write:", err)
					return
				}
				t.Log("srv write:", out.Wrote())
				closeErr := conn.Close()
				if closeErr != nil {
					t.Error("srv close:", closeErr)
				}
			})
		})
	})

	ctx = withWG(ctx)
	wgAdd(ctx)
	//
	rio.Dial(ctx, "tcp", "127.0.0.1:9000").OnComplete(func(ctx context.Context, conn rio.Connection, err error) {
		defer wgDone(ctx)
		if err != nil {
			t.Error("cli dial:", err)
			return
		}
		wgAdd(ctx)
		conn.Write([]byte("hello word")).OnComplete(func(ctx context.Context, out transport.Outbound, err error) {
			defer wgDone(ctx)
			if err != nil {
				t.Error("cli write:", err)
				return
			}
			t.Log("cli write:", out.Wrote())
			wgAdd(ctx)
			conn.Read().OnComplete(func(ctx context.Context, in transport.Inbound, err error) {
				defer wgDone(ctx)
				if err != nil {
					t.Error("cli read:", err)
					return
				}
				t.Log("cli read:", in.Received(), string(in.Reader().Peek(in.Received())))
			})
		})
	})

	wgWait(ctx)

}
