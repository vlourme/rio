package aio_test

import (
	"github.com/brickingsoft/rio/pkg/aio"
	"net"
	"sync"
	"testing"
	"time"
)

func TestConnect(t *testing.T) {
	ln, lnErr := net.Listen("tcp", ":9000")
	if lnErr != nil {
		t.Error("ln failed:", lnErr)
		return
	}
	defer ln.Close()

	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func(wg *sync.WaitGroup) {
		defer wg.Done()
		conn, connErr := ln.Accept()
		if connErr != nil {
			t.Error("conn failed:", connErr)
			return
		}
		t.Log("accept:", conn.LocalAddr(), conn.RemoteAddr())
		conn.Close()
		return
	}(wg)

	time.Sleep(time.Second)

	wg.Add(1)
	go aio.Connect("tcp", "127.0.0.1:9000", aio.ConnectOptions{}, func(result int, userdata aio.Userdata, err error) {
		defer wg.Done()
		if err != nil {
			t.Error("connect failed:", err)
			return
		}
		fd := userdata.Fd.(aio.NetFd)
		t.Log("connect:", result, fd.Fd(), fd.LocalAddr(), fd.RemoteAddr())
		wg.Add(1)
		go aio.Close(fd, func(result int, userdata aio.Userdata, err error) {
			wg.Done()
		})

	})

	wg.Wait()
	time.Sleep(time.Second)
}
