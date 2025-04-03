package rio_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/brickingsoft/rio"
	"io"
	"net"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestTCP(t *testing.T) {
	ctx := context.Background()
	config := rio.ListenConfig{
		MultipathTCP: false,
		ReusePort:    false,
	}
	ln, lnErr := config.Listen(ctx, "tcp", ":9000")
	if lnErr != nil {
		t.Error(lnErr)
		return
	}

	t.Log("ln addr:", ln.Addr())

	wg := new(sync.WaitGroup)
	defer wg.Wait()

	defer func() {
		err := ln.Close()
		if err != nil {
			t.Error(err)
		}
		return
	}()

	src := make([]byte, 4096*64)
	_, _ = rand.Read(src)

	wg.Add(1)
	go func(ln net.Listener, src []byte, wg *sync.WaitGroup) {
		defer wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				t.Error("accept", err)
				return
			}
			wg.Add(1)
			go func(conn net.Conn, src []byte, wg *sync.WaitGroup) {
				defer wg.Done()
				t.Log("srv:", conn.LocalAddr(), conn.RemoteAddr())

				b := make([]byte, len(src))

				var rn int
				for {
					rnn, rErr := conn.Read(b[rn:])
					if rErr != nil {
						if errors.Is(rErr, io.EOF) {
							t.Log("srv read EOF")
							return
						}
						t.Error("srv read failed", rnn, rErr)
						return
					}
					rn += rnn
					if rn == len(b) {
						break
					}
				}
				t.Log("srv read succeed", rn, bytes.Equal(src, b))

				var wn int
				for {
					wnn, wErr := conn.Write(b[wn:])
					if wErr != nil {
						t.Error("srv write failed", wnn, wErr)
						return
					}
					wn += wnn
					if wn == len(b) {
						break
					}
				}
				t.Log("srv write succeed", wn)

				_ = conn.Close()
			}(conn, src, wg)
			return
		}
	}(ln, src, wg)

	dialer := rio.DefaultDialer
	conn, connErr := dialer.Dial("tcp", "127.0.0.1:9000")
	if connErr != nil {
		t.Error(connErr)
		return
	}
	t.Log("cli:", conn.LocalAddr(), conn.RemoteAddr())
	defer time.Sleep(500 * time.Millisecond)
	defer conn.Close()

	var wn int
	for {
		wnn, wErr := conn.Write(src[wn:])
		if wErr != nil {
			t.Error("cli write failed", wn, wErr)
			return
		}
		wn += wnn
		if wn == len(src) {
			break
		}
	}
	t.Log("cli write succeed", wn)

	var rn int
	p := make([]byte, len(src))
	for {
		rnn, rErr := conn.Read(p[rn:])
		if rErr != nil {
			t.Error("cli read failed", rn, rErr)
			return
		}
		rn += rnn
		if rn == len(src) {
			break
		}
	}
	same := bytes.Equal(src, p)
	if same {
		t.Log("cli read succeed", rn, same)
	} else {
		t.Error("cli read failed", wn, rn)
	}

}

func TestTCPMultiAccept(t *testing.T) {
	ln, lnErr := rio.Listen("tcp", ":9000")
	if lnErr != nil {
		t.Error(lnErr)
		return
	}
	wg := new(sync.WaitGroup)
	defer wg.Wait()

	wg.Add(1)
	go func(ln net.Listener, wg *sync.WaitGroup) {
		defer wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				t.Error("accept", err)
				return
			}
			go func(conn net.Conn) {
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
				if wErr != nil {
					_ = conn.Close()
				}
			}(conn)
		}
	}(ln, wg)

	time.Sleep(500 * time.Millisecond)

	for i := 0; i < 2; i++ {
		t.Log("----------" + strconv.Itoa(i) + "----------")
		conn, connErr := rio.Dial("tcp", "127.0.0.1:9000")
		if connErr != nil {
			t.Error(connErr)
			return
		}
		t.Log("cli:", conn.LocalAddr(), conn.RemoteAddr())

		wn, wErr := conn.Write([]byte("hello world"))
		if wErr != nil {
			conn.Close()
			t.Error(wErr)
			return
		}
		t.Log("cli write:", wn)

		b := make([]byte, 1024)
		rn, rErr := conn.Read(b)
		t.Log("cli read", rn, string(b[:rn]), rErr)
		if rErr != nil {
			conn.Close()
			t.Error(rErr)
			return
		}
		conn.Close()
	}

	ln.Close()
}

func TestTCPConn_ReadFrom(t *testing.T) {
	// file
	//n := 1024 * 100
	n := 1024 * 42
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
	remain, remainErr := io.ReadAll(tmp)
	t.Log("remain:", len(remain), remainErr)

}

func TestConnection_SetReadTimeout(t *testing.T) {
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
			rc := conn.(rio.Conn)
			t.Log(rc.SendZCEnable())
			_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			t.Log("srv:", conn.LocalAddr(), conn.RemoteAddr())
			b := make([]byte, 1024)
			now := time.Now()
			rn, rErr := conn.Read(b)
			t.Log("srv read", time.Now().Sub(now), rn, string(b[:rn]), rErr, errors.Is(rErr, context.DeadlineExceeded))

			_ = conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
			wn, wnErr := conn.Write([]byte("hello world"))
			t.Log("srv write", wn, wnErr)
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

	time.Sleep(1 * time.Second)
	b := make([]byte, 1024)
	rn, rErr := conn.Read(b)
	t.Log("cli read", rn, rErr, string(b[:rn]))
	t.Log("done")
}
