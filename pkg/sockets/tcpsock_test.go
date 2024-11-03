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
	_ = conn.Close()
	wg.Wait()
}

func TestTcpConnection_ReadAndWrite(t *testing.T) {
	ln, err := sockets.ListenTCP("tcp", "127.0.0.1:9000", sockets.Options{})
	if err != nil {
		t.Fatal(err)
		return
	}
	defer ln.Close()
	wg := new(sync.WaitGroup)
	wg.Add(1)
	ln.Accept(func(conn sockets.Connection, err error) {
		if err != nil {
			t.Error("srv: accept ->", err)
			return
		}
		t.Log("srv: accepted!!!")
		p := make([]byte, 1024)
		conn.Read(p, func(n int, err error) {
			t.Log("srv: read ->", n, string(p[:n]), err)
			p = []byte(time.Now().String())
			conn.Write(p, func(n int, err error) {
				t.Log("srv: write ->", n, err)
				wg.Done()
			})
		})
	})
	conn, dialErr := net.Dial("tcp", "127.0.0.1:9000")
	if dialErr != nil {
		t.Error("cli: dial ->", dialErr)
		return
	}
	n, wErr := conn.Write([]byte("hello world"))
	t.Log("cli: write ->", n, wErr)
	p := make([]byte, 1024)
	n, rErr := conn.Read(p)
	t.Log("cli: read ->", n, string(p[0:n]), rErr)
	_ = conn.Close()
	wg.Wait()
}
