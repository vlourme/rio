package rio_test

import (
	"github.com/brickingsoft/rio"
	"net"
	"sync"
	"testing"
)

func TestUDP(t *testing.T) {
	rio.UseZeroCopy(true)
	rio.UseSides(1)

	srv, srvErr := rio.ListenPacket("udp", ":9000")
	if srvErr != nil {
		t.Error(srvErr)
		return
	}

	wg := new(sync.WaitGroup)

	wg.Add(1)
	go func(conn net.PacketConn, wg *sync.WaitGroup) {
		defer wg.Done()
		t.Log("srv:", conn.LocalAddr())
		b := make([]byte, 1024)
		rn, addr, rErr := srv.ReadFrom(b)
		t.Log("srv read", rn, string(b[:rn]), addr, rErr)
		if rErr != nil {
			_ = conn.Close()
			return
		}
		wn, wErr := conn.WriteTo(b[:rn], addr)
		t.Log("srv write", wn, wErr)
		_ = conn.Close()
		t.Log("srv done")
		return
	}(srv, wg)

	cli, cliErr := rio.Dial("udp", "127.0.0.1:9000")
	if cliErr != nil {
		t.Error(cliErr)
		return
	}
	t.Log("cli:", cli.LocalAddr(), cli.RemoteAddr())

	wn, wErr := cli.Write([]byte("hello world"))
	if wErr != nil {
		cli.Close()
		t.Error(wErr)
		return
	}
	t.Log("cli write:", wn)

	b := make([]byte, 1024)
	rn, rErr := cli.Read(b)
	t.Log("cli read", rn, string(b[:rn]), rErr)
	if rErr != nil {
		cli.Close()
		t.Error(rErr)
		return
	}
	cli.Close()
	t.Log("cli done")
	wg.Wait()
	t.Log("fin")
}
