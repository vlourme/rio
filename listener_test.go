package rio_test

import (
	"context"
	"github.com/brickingsoft/rio"
	"net"
	"testing"
)

func TestListen(t *testing.T) {
	ctx := context.Background()
	ln, lnErr := rio.Listen(ctx, "tcp", ":9000", rio.WithParallelAcceptors(1))
	if lnErr != nil {
		t.Error(lnErr)
		return
	}
	ln.Accept().OnComplete(func(ctx context.Context, conn rio.Connection, err error) {
		var addr net.Addr
		if conn != nil {
			addr = conn.RemoteAddr()
		}
		t.Log("accepted:", addr, err, ctx.Err())
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
