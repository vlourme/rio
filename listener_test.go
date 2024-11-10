package rio_test

import (
	"context"
	"github.com/brickingsoft/rio"
	"github.com/brickingsoft/rio/pkg/rate/timeslimiter"
	"net"
	"testing"
)

func TestListen(t *testing.T) {
	ctx := context.Background()
	ln, lnErr := rio.Listen(
		ctx,
		"tcp", ":9000",
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
