package rio_test

import (
	"context"
	"github.com/brickingsoft/rio"
	"github.com/brickingsoft/rio/pkg/rate/timeslimiter"
	"github.com/brickingsoft/rio/transport"
	"net"
	"testing"
)

func TestListenTCP(t *testing.T) {
	ctx := context.Background()
	ln, lnErr := rio.Listen(
		ctx,
		"tcp", ":9000",
		rio.ParallelAcceptors(1),
		rio.MaxConnections(10),
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

func TestTCP(t *testing.T) {
	ctx := context.Background()
	ln, lnErr := rio.Listen(ctx, "tcp", ":9000", rio.ParallelAcceptors(1))
	if lnErr != nil {
		t.Error(lnErr)
		return
	}
	defer ln.Close()

	ln.Accept().OnComplete(func(ctx context.Context, conn rio.Connection, err error) {
		if err != nil {
			t.Error("srv accept:", rio.IsClosed(err), err)
			return
		}
		t.Log("srv accept:", conn.RemoteAddr(), err)
		conn.Read().OnComplete(func(ctx context.Context, in transport.Inbound, err error) {
			if err != nil {
				t.Error("srv read:", err)
				_ = conn.Close()
				return
			}
			// todo fix future bug
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

	//executors := rxp.New()
	//defer executors.Close()
	//ctx = rxp.With(ctx, executors)
	//
	//rio.Dial(ctx, "tcp", "127.0.0.1:9000").OnComplete(func(ctx context.Context, conn rio.Connection, err error) {
	//	if err != nil {
	//		t.Error("dial:", err)
	//		return
	//	}
	//	conn.Write([]byte("hello world")).OnComplete(func(ctx context.Context, out transport.Outbound, err error) {
	//		if err != nil {
	//			t.Error("cli rite:", err)
	//			return
	//		}
	//		t.Log("write:", out.Wrote())
	//		conn.Read().OnComplete(func(ctx context.Context, in transport.Inbound, err error) {
	//			if err != nil {
	//				t.Error("cli read:", err)
	//				return
	//			}
	//			t.Log("cli read:", in.Received(), string(in.Reader().Peek(100)))
	//			return
	//		})
	//	})
	//})
	//
	//time.Sleep(1 * time.Second)

	conn, dialErr := net.Dial("tcp", ":9000")
	if dialErr != nil {
		t.Error(dialErr)
		return
	}
	defer conn.Close()
	wn, wErr := conn.Write([]byte("hello world"))
	if wErr != nil {
		t.Error("client write:", wErr)
		return
	}
	t.Log("client write:", wn)
	p := make([]byte, 1024)
	rn, rErr := conn.Read(p)
	if rErr != nil {
		t.Error("client read:", rErr)
		return
	}
	t.Log("client read:", rn, string(p))
}
