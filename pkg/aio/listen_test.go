package aio_test

import (
	"github.com/brickingsoft/rio/pkg/aio"
	"net"
	"sync"
	"testing"
	"time"
)

func TestAccept(t *testing.T) {
	aio.Startup(aio.Options{
		Cylinders: 1,
		Settings:  nil,
	})
	defer aio.Shutdown()
	lnFd, lnErr := aio.Listen("tcp", ":9000", aio.ListenerOptions{})
	if lnErr != nil {
		t.Error("ln failed:", lnErr)
		return
	}

	wg := new(sync.WaitGroup)
	loops := 3

	for i := 0; i < loops; i++ {
		wg.Add(1)
		go aio.Accept(lnFd, func(result int, userdata aio.Userdata, err error) {
			defer wg.Done()
			if err != nil {
				t.Error("srv accept failed:", result, err)
				return
			}
			connFd := userdata.Fd.(aio.NetFd)
			t.Log("srv accept:", result, "fd:", connFd.Fd(), "addr:", connFd.LocalAddr(), "->", connFd.RemoteAddr())

			wg.Add(1)
			go aio.Close(connFd, func(result int, userdata aio.Userdata, err error) {
				defer wg.Done()
				t.Log("srv close:", result, err)
			})
		})
	}

	conns := make([]net.Conn, 0, 1)
	for i := 0; i < loops; i++ {
		conn, err := net.Dial("tcp", "127.0.0.1:9000")
		if err != nil {
			t.Error("dial failed:", err)
			return
		}
		time.Sleep(10 * time.Millisecond)
		conns = append(conns, conn)
	}
	wg.Wait()
	for _, conn := range conns {
		_ = conn.Close()
	}
	wg.Add(1)
	go aio.Close(lnFd, func(result int, userdata aio.Userdata, err error) {
		wg.Done()
	})
	wg.Wait()
}

func TestListen(t *testing.T) {
	aio.Startup(aio.Options{})
	defer aio.Shutdown()
	_, lnErr := aio.Listen("udp", ":9000", aio.ListenerOptions{
		MultipathTCP:       false,
		MulticastInterface: nil,
	})
	if lnErr != nil {
		t.Error("ln failed:", lnErr)
		return
	}
}
