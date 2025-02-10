package adaptor_test

import (
	"github.com/brickingsoft/rio"
	"github.com/brickingsoft/rio/transport/adaptor"
	"net"
	"sync"
	"testing"
)

func TestPacket(t *testing.T) {
	rio.Startup()
	defer rio.Shutdown()

	ln, lnErr := rio.ListenPacket("udp", ":9000")
	if lnErr != nil {
		t.Error(lnErr)
		return
	}
	srv := adaptor.Packet(ln)

	wg := new(sync.WaitGroup)
	defer wg.Wait()

	wg.Add(1)
	go func(srv net.PacketConn, wg *sync.WaitGroup) {
		defer wg.Done()
		b := make([]byte, 1024)
		rn, addr, rErr := srv.ReadFrom(b)
		if rErr != nil {
			t.Error("srv read error", rErr)
			_ = srv.Close()
			return
		}
		t.Log("srv read", rn, addr, string(b[:rn]))

		wn, wErr := srv.WriteTo(b[:rn], addr)
		if wErr != nil {
			t.Error("srv write error", wErr)
			_ = srv.Close()
			return
		}
		t.Log("srv write", wn)
		_ = srv.Close()
		return
	}(srv, wg)

	conn, dialErr := net.Dial("udp", "127.0.0.1:9000")
	if dialErr != nil {
		t.Error(dialErr)
		_ = srv.Close()
		return
	}

	wn, wErr := conn.Write([]byte("hello world"))
	if wErr != nil {
		t.Error("client write error:", wErr)
		_ = conn.Close()
		_ = srv.Close()
		return
	}
	t.Log("client write:", wn)

	b := make([]byte, 1024)
	rn, rErr := conn.Read(b)
	if rErr != nil {
		t.Error("client read error:", rErr)
		_ = conn.Close()
		_ = srv.Close()
		return
	}

	t.Log("client read:", rn, string(b[:rn]))
	_ = conn.Close()
	_ = srv.Close()

}
