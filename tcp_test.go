package rio_test

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/brickingsoft/rio"
	"io"
	"net"
	"os"
	"sync"
	"testing"
)

func TestTCP(t *testing.T) {
	rio.UseZeroCopy(true)

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
		for {
			conn, err := ln.Accept()
			if err != nil {
				t.Error("accept", err)
				return
			}
			t.Log("srv:", conn.LocalAddr(), conn.RemoteAddr())
			b := make([]byte, 1024)
			rn, rErr := conn.Read(b)
			t.Log("srv read", rn, string(b[:rn]), rErr)
			if rErr != nil {
				_ = conn.Close()
				return
			}
			wn, wErr := conn.Write(b[:rn])
			t.Log("srv write", wn, wErr)
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

func TestTCPConn_ReadFrom(t *testing.T) {
	// file
	n := 1024 * 100
	rb := make([]byte, n)
	bLen, bErr := rand.Read(rb)
	if bErr != nil {
		t.Error(bErr)
		return
	}
	if bLen != n {
		t.Error(bLen, n)
		return
	}
	b := make([]byte, hex.EncodedLen(len(rb)))
	hex.Encode(b, rb)

	tmp, tmpErr := os.CreateTemp("", "rio_*.txt")
	if tmpErr != nil {
		t.Error(tmpErr)
		return
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	tn, tErr := tmp.Write(b)
	if tErr != nil {
		t.Error("tmp write", tErr)
		return
	}
	if tn != len(b) {
		t.Error("tmp write", tn, len(b))
		return
	}
	_, _ = tmp.Seek(0, io.SeekStart)

	// srv
	ln, lnErr := rio.Listen("tcp", ":9000")
	if lnErr != nil {
		t.Error(lnErr)
		return
	}
	defer ln.Close()
	wg := new(sync.WaitGroup)
	defer wg.Wait()
	wg.Add(1)
	go func(ln net.Listener, wg *sync.WaitGroup) {
		defer wg.Done()

		srv, srvErr := ln.Accept()
		if srvErr != nil {
			t.Error("accept", srvErr)
			return
		}
		defer srv.Close()

		buf := bytes.NewBuffer(nil)
		srb := make([]byte, 1024*10)
		for {
			rn, rErr := srv.Read(srb)
			if rErr != nil {
				if errors.Is(rErr, io.EOF) {
					break
				}
				t.Error("srv read", rn, rErr)
				return
			}
			buf.Write(srb[:rn])
		}
		t.Log("srv:", buf.Len(), bytes.Equal(b, buf.Bytes()))
	}(ln, wg)

	// cli
	cli0, cliErr := rio.Dial("tcp", "127.0.0.1:9000")
	if cliErr != nil {
		t.Error("dial", cliErr)
		return
	}
	defer cli0.Close()

	cli := cli0.(io.ReaderFrom)
	crn, crErr := cli.ReadFrom(tmp)
	if crErr != nil {
		t.Error("cli read from", crErr)
		return
	}
	t.Log("cli read from", crn, crn == int64(len(b)))

}

func TestConnection_SetReadTimeout(t *testing.T) {

}
