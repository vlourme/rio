package sockets_test

import (
	"github.com/brickingsoft/rio/pkg/sockets"
	"net"
	"sync"
	"testing"
	"time"
)

func TestListenTCP(t *testing.T) {
	ln, err := sockets.Listen("tcp", "127.0.0.1:9000", sockets.Options{})
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
	ln, err := sockets.Listen("tcp", "127.0.0.1:9000", sockets.Options{})
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

func TestConnection_ReadAndWrite(t *testing.T) {
	ln, err := sockets.Listen("tcp", "127.0.0.1:9000", sockets.Options{})
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
		go func(conn sockets.Connection, wg *sync.WaitGroup) {
			p := make([]byte, 1024)
			conn.Read(p, func(n int, err error) {
				t.Log("srv: read ->", n, string(p[:n]), err)
				go func(conn sockets.Connection, wg *sync.WaitGroup) {
					conn.Write([]byte(time.Now().String()), func(n int, err error) {
						t.Log("srv: write ->", n, err)
						wg.Done()
					})
				}(conn, wg)
			})
		}(conn, wg)
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

func TestDialTCP(t *testing.T) {
	ln, err := sockets.Listen("tcp", "127.0.0.1:9000", sockets.Options{})
	if err != nil {
		t.Fatal(err)
		return
	}
	defer func() {
		err = ln.Close()
		if err != nil {
			t.Fatal(err)
			return
		}
	}()
	wg := new(sync.WaitGroup)
	wg.Add(1)
	ln.Accept(func(conn sockets.Connection, err error) {
		defer wg.Done()
		if err != nil {
			t.Error("srv: accept ->", err)
			return
		}
		t.Log("srv: accepted!!!", conn.LocalAddr(), conn.RemoteAddr())
	})
	wg.Add(1)
	sockets.Dial("tcp", "127.0.0.1:9000", sockets.Options{}, func(conn sockets.Connection, err error) {
		defer wg.Done()
		if err != nil {
			t.Error("cli: dial ->", err)
			return
		}
		t.Log("cli: dialed!!!", conn.LocalAddr(), conn.RemoteAddr())
		closeErr := conn.Close()
		if closeErr != nil {
			t.Error("cli: dial ->", closeErr)
		}
		return
	})
	wg.Wait()
}
