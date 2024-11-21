package rio_test

import (
	"context"
	"github.com/brickingsoft/rio"
	"github.com/brickingsoft/rio/pkg/rate/timeslimiter"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"net"
	"sync"
	"testing"
	"time"
)

func TestListenTCP(t *testing.T) {
	ctx := context.Background()
	ctx = withWG(ctx)

	ln, lnErr := rio.Listen(
		ctx,
		"tcp", "127.0.0.1:9000",
		rio.WithStreamListenerParallelAcceptors(1),
		rio.WithStreamListenerAcceptMaxConnections(5),
	)
	if lnErr != nil {
		t.Error(lnErr)
		return
	}

	wgAdd(ctx)
	ln.Accept().OnComplete(func(ctx context.Context, conn rio.Connection, err error) {
		if err != nil {
			t.Log("accepted:", timeslimiter.Tokens(ctx), rio.IsClosed(err), err, ctx.Err())
			wgDone(ctx)
			return
		}

		var addr net.Addr
		if conn != nil {
			addr = conn.RemoteAddr()
		}
		t.Log("accepted:", timeslimiter.Tokens(ctx), addr, err, ctx.Err())
		wgAdd(ctx)
		if conn != nil {
			conn.Close().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
				if cause != nil {
					t.Error("srv close conn err:", cause)
				}
				wgDone(ctx)
			})
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

	wgAdd(ctx)
	ln.Close().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
		if cause != nil {
			t.Error("ln close close err:", cause)
		}
		wgDone(ctx)
	})
	wgWait(ctx)
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
	_ = rio.Startup()
	defer rio.ShutdownGracefully()

	ctx := context.Background()
	ctx = withWG(ctx)

	ln, lnErr := rio.Listen(ctx,
		"tcp", ":9000",
		rio.WithStreamListenerParallelAcceptors(10),
		rio.WithPromiseMakeOptions(async.WithDirectMode()),
	)
	if lnErr != nil {
		t.Error(lnErr)
		return
	}

	wgAdd(ctx)
	ln.Accept().OnComplete(func(ctx context.Context, conn rio.Connection, err error) {
		if err != nil {
			if rio.IsClosed(err) {
				t.Log("srv accept closed")
			} else {
				t.Error("srv accept:", rio.IsClosed(err), err)
			}
			return
		}
		wgDone(ctx)

		t.Log("srv accept:", conn.RemoteAddr(), err)

		_ = conn.SetReadTimeout(500 * time.Millisecond)

		wgAdd(ctx)
		conn.Read().OnComplete(func(ctx context.Context, in transport.Inbound, err error) {
			defer wgDone(ctx)
			if err != nil {
				t.Error("srv read:", err)
				wgAdd(ctx)
				conn.Close().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
					defer wgDone(ctx)
				})
				return
			}
			n := in.Received()
			p, _ := in.Reader().Next(n)
			t.Log("srv read:", n, string(p))
			wgAdd(ctx)
			conn.Write(p).OnComplete(func(ctx context.Context, out transport.Outbound, err error) {
				defer wgDone(ctx)
				if err != nil {
					t.Error("srv write:", err)
					return
				}
				t.Log("srv write:", out.Wrote())
				wgAdd(ctx)
				conn.Close().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
					t.Log("srv close:", cause)
					wgDone(ctx)
				})
			})
		})
	})

	wgAdd(ctx)
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
				wgAdd(ctx)
				conn.Close().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
					defer wgDone(ctx)
					t.Log("cli close:", err)
				})
			})
		})
	})

	wgWait(ctx)
	wgAdd(ctx)
	ln.Close().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
		t.Log("ln close:", cause)
		wgDone(ctx)
	})
	wgWait(ctx)
}
