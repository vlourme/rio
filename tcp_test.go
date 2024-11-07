package rio_test

import (
	"context"
	"github.com/brickingsoft/rio"
	"net"
	"testing"
)

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
				return
			}
			t.Error("srv accept:", err)
			return
		}
		t.Log("srv accept:", conn.RemoteAddr(), err)
		conn.Read().OnComplete(func(ctx context.Context, in rio.Inbound, err error) {
			if err != nil {
				t.Error("srv read:", err)
				_ = conn.Close()
				return
			}
			n := in.Received()
			p, _ := in.Buffer().Next(n)
			t.Log("srv read:", n, string(p))
			conn.Write(p).OnComplete(func(ctx context.Context, out rio.Outbound, err error) {
				if err != nil {
					t.Error("srv write:", err)
					return
				}
				t.Log("srv write:", out.Wrote())
			})
		})
	})
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
