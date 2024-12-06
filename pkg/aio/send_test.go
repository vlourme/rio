package aio_test

import (
	"context"
	"github.com/brickingsoft/rio/pkg/aio"
	"net"
	"sync"
	"testing"
)

func TestSend(t *testing.T) {
	aio.Startup(aio.Options{})
	defer aio.Shutdown()
	ln, lnErr := net.Listen("tcp", ":9000")
	if lnErr != nil {
		t.Error(lnErr)
		return
	}
	wg := new(sync.WaitGroup)
	ctx, cancel := context.WithCancel(context.Background())

	go func(ctx context.Context, ln net.Listener) {
		defer wg.Done()
		<-ctx.Done()
		ln.Close()
	}(ctx, ln)

	wg.Add(1)
	go func(ln net.Listener) {
		defer wg.Done()
		conn, err := ln.Accept()
		if err != nil {
			t.Error(err)
			return
		}
		b := make([]byte, 1024)
		n, rErr := conn.Read(b)
		if rErr != nil {
			t.Error(rErr)
			return
		}
		t.Log("recv:", string(b[:n]))
	}(ln)

	wg.Add(1)
	go aio.Connect("tcp", "127.0.0.1:9000", aio.ConnectOptions{}, func(result int, userdata aio.Userdata, err error) {
		defer wg.Done()
		if err != nil {
			wg.Add(1)
			cancel()
			t.Error(err)
			return
		}
		fd := userdata.Fd.(aio.NetFd)
		wg.Add(1)
		go aio.Send(fd, []byte("hello world"), func(result int, userdata aio.Userdata, err error) {
			defer wg.Done()
			if err != nil {
				wg.Add(1)
				cancel()
				t.Error(err)
				return
			}
			t.Log("send:", result)
		})
	})

	wg.Wait()
	wg.Add(1)
	cancel()
	wg.Wait()
}

func TestSendTo(t *testing.T) {
	aio.Startup(aio.Options{})
	defer aio.Shutdown()
	srv, srvErr := net.ListenUDP("udp", &net.UDPAddr{
		IP:   net.ParseIP("0.0.0.0"),
		Port: 9000,
	})
	if srvErr != nil {
		t.Error(srvErr)
		return
	}

	wg := new(sync.WaitGroup)
	ctx, cancel := context.WithCancel(context.Background())

	go func(ctx context.Context, srv *net.UDPConn) {
		defer wg.Done()
		<-ctx.Done()
		srv.Close()
	}(ctx, srv)

	wg.Add(1)
	go func(srv *net.UDPConn) {
		defer wg.Done()
		b := make([]byte, 1024)
		n, addr, rErr := srv.ReadFrom(b)
		if rErr != nil {
			t.Error(rErr)
			return
		}
		t.Log("recv:", string(b[:n]), addr)
	}(srv)

	wg.Add(1)
	go aio.Connect("udp", "127.0.0.1:9000", aio.ConnectOptions{}, func(result int, userdata aio.Userdata, err error) {
		defer wg.Done()
		if err != nil {
			t.Error(err)
			wg.Add(1)
			cancel()

			return
		}
		fd := userdata.Fd.(aio.NetFd)
		wg.Add(1)
		go aio.Send(fd, []byte("hello world"), func(result int, userdata aio.Userdata, err error) {
			defer wg.Done()
			if err != nil {
				wg.Add(1)
				cancel()
				t.Error(err)
				return
			}
			t.Log("send:", result)
		})
	})

	wg.Wait()
	wg.Add(1)
	cancel()
	wg.Wait()
}
