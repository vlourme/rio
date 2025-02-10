package adaptor_test

import (
	"github.com/brickingsoft/rio"
	"github.com/brickingsoft/rio/transport/adaptor"
	"github.com/brickingsoft/rxp/async"
	"net"
	"sync"
	"testing"
)

func TestConnection(t *testing.T) {
	rio.Startup()
	defer rio.Shutdown()

	ln, lnErr := rio.Listen(
		"tcp", "127.0.0.1:9000",
		rio.WithParallelAcceptors(3),
	)
	if lnErr != nil {
		t.Error(lnErr)
		return
	}

	netLn := adaptor.Listener(ln)

	wg := new(sync.WaitGroup)
	defer wg.Wait()

	wg.Add(1)
	go func(ln net.Listener, wg *sync.WaitGroup) {
		for {
			conn, err := ln.Accept()
			if err != nil {
				if async.IsCanceled(err) {
					break
				}
				t.Log(err)
				break
			}
			t.Log("accepted:", conn.LocalAddr(), conn.RemoteAddr())
			b := make([]byte, 1024)
			rn, rErr := conn.Read(b)
			if rErr != nil {
				t.Error("srv read error:", rErr)
				conn.Close()
				continue
			}
			t.Log("srv read:", rn, string(b[:rn]))

			wn, wnErr := conn.Write(b[:rn])
			if wnErr != nil {
				t.Error("srv write error:", wnErr)
				conn.Close()
				continue
			}
			t.Log("wrote:", wn)
			conn.Close()
		}
		wg.Done()
	}(netLn, wg)

	conn, dialErr := net.Dial("tcp", "127.0.0.1:9000")
	if dialErr != nil {
		t.Error(dialErr)
		return
	}

	wn, wErr := conn.Write([]byte("hello world"))
	if wErr != nil {
		t.Error("client write error:", wErr)
		_ = conn.Close()
		_ = netLn.Close()
		return
	}
	t.Log("client write:", wn)

	b := make([]byte, 1024)
	rn, rErr := conn.Read(b)
	if rErr != nil {
		t.Error("client read error:", rErr)
		_ = conn.Close()
		_ = netLn.Close()
		return
	}

	t.Log("client read:", rn, string(b[:rn]))
	_ = conn.Close()
	_ = netLn.Close()
	return
}
