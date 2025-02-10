package adaptor_test

import (
	"github.com/brickingsoft/rio"
	"github.com/brickingsoft/rio/transport/adaptor"
	"github.com/brickingsoft/rxp/async"
	"net"
	"sync"
	"testing"
)

func TestListener(t *testing.T) {
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
			_ = conn.Close()
		}
		wg.Done()
	}(netLn, wg)

	for i := 0; i < 3; i++ {
		conn, dialErr := net.Dial("tcp", "127.0.0.1:9000")
		if dialErr != nil {
			t.Error(dialErr)
			return
		}
		_ = conn.Close()
	}

	_ = netLn.Close()

	wg.Wait()

}
