package rio_test

import (
	"context"
	"github.com/brickingsoft/rio"
	"net"
	"sync/atomic"
	"testing"
)

func TestListen(t *testing.T) {
	ctx := context.Background()
	ln, lnErr := rio.Listen(ctx, "tcp", ":9000", rio.WithLoops(4))
	if lnErr != nil {
		t.Error(lnErr)
		return
	}
	defer ln.Close()
	count := atomic.Int64{}
	ln.Accept().OnComplete(func(ctx context.Context, conn rio.Connection, err error) {
		var addr net.Addr
		if conn != nil {
			addr = conn.RemoteAddr()
		}
		t.Log("accepted:", count.Add(1), addr, err)
	})
	for i := 0; i < 10; i++ {
		conn, dialErr := net.Dial("tcp", ":9000")
		if dialErr != nil {
			t.Error(dialErr)
			return
		}
		t.Log("dialed:", i+1, conn.LocalAddr())
		_ = conn.Close()
	}
}
