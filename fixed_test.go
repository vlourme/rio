package rio_test

import (
	"github.com/brickingsoft/rio"
	"net"
	"os"
	"sync"
	"testing"
)

func TestFixed(t *testing.T) {
	os.Setenv("RIO_IOURING_REG_FIXED_BUFFERS", "1024,10")
	os.Setenv("RIO_IOURING_REG_FIXED_FILES", "10")

	ln, lnErr := rio.Listen("tcp", ":9000")
	if lnErr != nil {
		t.Error(lnErr)
		return
	}
	wg := new(sync.WaitGroup)
	defer wg.Wait()

	defer func() {
		err := ln.Close()
		if err != nil {
			t.Error(err)
		}
		return
	}()

	wg.Add(1)
	go func(ln net.Listener, wg *sync.WaitGroup) {
		defer wg.Done()
		b := make([]byte, 1024)
		for {
			conn, err := ln.Accept()
			if err != nil {
				t.Error("accept", err)
				return
			}
			t.Log("srv:", conn.LocalAddr(), conn.RemoteAddr())
			frw, _ := rio.ConvertToFixedReaderWriter(conn)
			buf := frw.AcquireRegisteredBuffer()
			rn, rErr := frw.ReadFixed(buf)
			_, _ = buf.Read(b[:rn])
			t.Log("srv read", rn, string(b[:rn]), rErr)
			if rErr != nil {
				frw.ReleaseRegisteredBuffer(buf)
				_ = conn.Close()
				return
			}
			buf.Reset()
			_, _ = buf.Write(b[:rn])
			wn, wErr := frw.WriteFixed(buf)
			t.Log("srv write", wn, wErr)
			frw.ReleaseRegisteredBuffer(buf)
			_ = conn.Close()
			return
		}
	}(ln, wg)

	conn, connErr := rio.Dial("tcp", "127.0.0.1:9000")
	if connErr != nil {
		t.Error(connErr)
		return
	}
	t.Log("cli:", conn.LocalAddr(), conn.RemoteAddr())
	defer conn.Close()

	wn, wErr := conn.Write([]byte("hello world"))
	if wErr != nil {
		t.Error(wErr)
		return
	}
	t.Log("cli write:", wn)

	b := make([]byte, 1024)
	rn, rErr := conn.Read(b)
	t.Log("cli read", rn, string(b[:rn]), rErr)
	if rErr != nil {
		t.Error(rErr)
		return
	}
}
