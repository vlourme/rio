package sockets_test

import (
	"github.com/brickingsoft/rio/pkg/sockets"
	"net"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestGetAddrAndFamily(t *testing.T) {
	aps := []string{
		"127.0.0.1:8080",
		":8080",
		"[::]:888",
		"[fe80::e243:1f44:1650:563e%23]:8080",
	}
	for _, network := range []string{"tcp", "tcp4", "tcp6", "udp", "udp4", "udp6"} {
		for _, ap := range aps {
			addr, family, ipv6, err := sockets.GetAddrAndFamily(network, ap)
			fn := "unknown"
			switch family {
			case syscall.AF_INET:
				fn = "AF_INET"
				break
			case syscall.AF_INET6:
				fn = "AF_INET6"
				break
			}
			t.Log(network, ap, "->", fn, addr, ipv6, err)
		}
	}
}

func TestListenTCP(t *testing.T) {
	ln, err := sockets.ListenTCP("tcp", "127.0.0.1:9000", sockets.Options{})
	if err != nil {
		t.Fatal(err)
		return
	}
	err = ln.Close()
	if err != nil {
		t.Fatal(err)
		return
	}
}

func TestTcpListener_Accept(t *testing.T) {
	ln, err := sockets.ListenTCP("tcp", "127.0.0.1:9000", sockets.Options{})
	if err != nil {
		t.Fatal(err)
		return
	}
	defer ln.Close()
	wg := new(sync.WaitGroup)
	wg.Add(1)
	ln.Accept(func(conn sockets.Connection, err error) {
		wg.Done()
		if err != nil {
			t.Error("accept ->", err)
			return
		}
		t.Log("accepted!!!")
		err = conn.Close()
		if err != nil {
			t.Error("accept close:", err)
		}
	})
	conn, dialErr := net.Dial("tcp", "127.0.0.1:9000")
	if dialErr != nil {
		t.Error("dial ->", dialErr)
		return
	}
	time.Sleep(3 * time.Second)
	_ = conn.Close()
	wg.Wait()
}
