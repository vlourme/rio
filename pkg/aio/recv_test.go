package aio_test

import (
	"bytes"
	"github.com/brickingsoft/rio/pkg/aio"
	"net"
	"sync"
	"testing"
	"time"
)

func TestRecv(t *testing.T) {
	aio.Startup(aio.Options{})
	defer aio.Shutdown()
	lnFd, lnErr := aio.Listen("tcp", ":9000", aio.ListenerOptions{})
	if lnErr != nil {
		t.Error("ln failed:", lnErr)
		return
	}

	wg := new(sync.WaitGroup)

	wg.Add(1)
	go aio.Accept(lnFd, func(result int, userdata aio.Userdata, err error) {
		defer wg.Done()
		if err != nil {
			t.Error("srv accept:", err)
			return
		}
		t.Log("srv accept:", result)
		connFd := userdata.Fd.(aio.NetFd)
		connFd.SetReadTimeout(500 * time.Millisecond)
		b := make([]byte, 1024)
		wg.Add(1)
		go aio.Recv(connFd, b, func(result int, userdata aio.Userdata, err error) {
			defer wg.Done()
			if err != nil {
				t.Error("srv read:", result, err)
				return
			}
			t.Log("srv read:", "param:", result, string(b[:result]))
			buf := userdata.Msg.Bytes(0)
			read := buf[:userdata.QTY]
			t.Log("srv read:", "userdata:", userdata.QTY, string(read))
			t.Log("srv read:", "same value:",
				"qty:", userdata.QTY == uint32(result),
				"buf:", bytes.Equal(read, b[:result]),
				"buf ptr:", &buf[0] == &b[0],
			)
		})
	})

	conn, connErr := net.Dial("tcp", "127.0.0.1:9000")
	if connErr != nil {
		t.Error("dial failed:", connErr)
		return
	}
	defer conn.Close()

	wn, wErr := conn.Write([]byte("hello world"))
	if wErr != nil {
		t.Error("write failed:", wErr)
		return
	}
	t.Log("cli write:", wn)
	wg.Wait()
}

func TestRecvFrom(t *testing.T) {
	aio.Startup(aio.Options{})
	defer aio.Shutdown()
	lnFd, lnErr := aio.Listen("udp", ":9000", aio.ListenerOptions{})
	if lnErr != nil {
		t.Error("ln failed:", lnErr)
		return
	}
	wg := new(sync.WaitGroup)

	wg.Add(1)
	b := make([]byte, 1024)
	go aio.RecvFrom(lnFd, b, func(result int, userdata aio.Userdata, err error) {
		defer wg.Done()
		if err != nil {
			t.Error("srv read:", err)
			return
		}
		raddr, addrErr := userdata.Msg.Addr()
		if addrErr != nil {
			t.Error("srv read:", addrErr)
			return
		}
		t.Log("srv read:", string(b[:result]), raddr)
	})

	conn, connErr := net.Dial("udp", "127.0.0.1:9000")
	if connErr != nil {
		t.Error("dial failed:", connErr)
		return
	}
	defer conn.Close()

	wn, wErr := conn.Write([]byte("hello world"))
	if wErr != nil {
		t.Error("write failed:", wErr)
		return
	}
	t.Log("cli write:", wn)
	wg.Wait()
}

func TestRecvMsg(t *testing.T) {
	aio.Startup(aio.Options{})
	defer aio.Shutdown()
	lnFd, lnErr := aio.Listen("udp", ":9000", aio.ListenerOptions{})
	if lnErr != nil {
		t.Error("ln failed:", lnErr)
		return
	}
	wg := new(sync.WaitGroup)

	wg.Add(1)
	b := make([]byte, 1024)
	oob := make([]byte, 1024)
	go aio.RecvMsg(lnFd, b, oob, func(result int, userdata aio.Userdata, err error) {
		defer wg.Done()
		if err != nil {
			t.Error("srv read:", err)
			return
		}
		raddr, addrErr := userdata.Msg.Addr()
		if addrErr != nil {
			t.Error("srv read:", addrErr)
			return
		}
		control := userdata.Msg.ControlBytes()
		t.Log("srv read:", string(b[:result]), raddr, len(control), string(control))
	})

	conn, connErr := net.Dial("udp", "127.0.0.1:9000")
	if connErr != nil {
		t.Error("dial failed:", connErr)
		return
	}
	defer conn.Close()

	udpc := conn.(*net.UDPConn)
	wn, woobn, wErr := udpc.WriteMsgUDP([]byte("hello world"), []byte("oob"), nil)
	if wErr != nil {
		t.Error("write failed:", wErr)
		return
	}

	t.Log("cli write:", wn, woobn)
	wg.Wait()
}
