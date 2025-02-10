package rio_test

import (
	"bytes"
	"context"
	"github.com/brickingsoft/rio"
	"github.com/brickingsoft/rio/transport"
	"github.com/brickingsoft/rxp/async"
	"net"
	"os"
	"sync"
	"testing"
	"time"
)

func TestListenTCP(t *testing.T) {
	rio.Startup()
	defer rio.Shutdown()

	ln, lnErr := rio.Listen(
		"tcp", "127.0.0.1:9000",
		rio.WithParallelAcceptors(3),
	)
	if lnErr != nil {
		t.Error(lnErr)
		return
	}

	lwg := new(sync.WaitGroup)
	lwg.Add(1)

	loops := 3
	awg := new(sync.WaitGroup)
	awg.Add(loops)
	ln.OnAccept(func(ctx context.Context, conn rio.Connection, err error) {
		if err != nil {
			if !async.IsCanceled(err) {
				t.Error("accepted failed:", err)
			}
			lwg.Done()
			return
		}

		var addr net.Addr
		if conn != nil {
			addr = conn.RemoteAddr()
		}
		t.Log("accepted:", addr, err, ctx.Err())
		if conn != nil {
			_ = conn.Close()
			awg.Done()
		}
	})

	for i := 0; i < loops; i++ {
		conn, dialErr := net.Dial("tcp", ":9000")
		if dialErr != nil {
			t.Error(dialErr)
			return
		}
		t.Log("dialed:", i+1, conn.LocalAddr())
		//time.Sleep(time.Millisecond * 100)
		err := conn.Close()
		if err != nil {
			t.Error("cli close conn err:", err)
		}
	}

	awg.Wait()

	_ = ln.Close()
	lwg.Wait()
}

func TestTCP(t *testing.T) {
	rio.Startup()
	defer rio.Shutdown()

	ctx := rio.Background()

	ln, lnErr := rio.Listen(
		"tcp", ":9000",
		rio.WithFastOpen(1),
	)
	if lnErr != nil {
		t.Error(lnErr)
		return
	}

	lwg := new(sync.WaitGroup)
	lwg.Add(1)

	swg := new(sync.WaitGroup)
	swg.Add(1)
	ln.OnAccept(func(ctx context.Context, conn rio.Connection, err error) {
		if err != nil {
			if async.IsCanceled(err) {
				t.Log("srv closed")
			} else {
				t.Error("srv accept failed:", err)
			}
			lwg.Done()
			return
		}

		t.Log("srv accept:", conn.RemoteAddr(), err)
		conn.Read().OnComplete(func(ctx context.Context, in transport.Inbound, err error) {
			if err != nil {
				t.Error("srv read:", err)
				_ = conn.Close()
				swg.Done()
				return
			}
			n := in.Len()
			p, _ := in.Next(n)
			t.Log("srv read:", n, string(p))
			conn.Write(p).OnComplete(func(ctx context.Context, out int, err error) {
				if err != nil {
					t.Error("srv write:", err)
					_ = conn.Close()
					swg.Done()
					return
				}
				t.Log("srv write:", out)
				_ = conn.Close()
				swg.Done()
			})
		})
	})

	cwg := new(sync.WaitGroup)
	cwg.Add(1)
	rio.Dial(ctx, "tcp", "127.0.0.1:9000").OnComplete(func(ctx context.Context, conn rio.Connection, err error) {
		if err != nil {
			t.Error("cli dial:", err)
			cwg.Done()
			return
		}

		conn.Write([]byte("hello word")).OnComplete(func(ctx context.Context, out int, err error) {
			if err != nil {
				t.Error("cli write:", err)
				cwg.Done()
				return
			}
			t.Log("cli write:", out)
			conn.Read().OnComplete(func(ctx context.Context, in transport.Inbound, err error) {
				if err != nil {
					t.Error("cli read:", err)
					cwg.Done()
					return
				}
				n := in.Len()
				p, _ := in.Next(n)
				t.Log("cli read:", string(p))
				_ = conn.Close()
				cwg.Done()
			})
		})
	})

	cwg.Wait()
	swg.Wait()

	_ = ln.Close()
	lwg.Wait()
}

func TestTcpConnection_Sendfile(t *testing.T) {
	file, fileErr := os.CreateTemp("", "rio_*.txt")
	if fileErr != nil {
		t.Error(fileErr)
		return
	}

	content := []byte("hello world")
	_, _ = file.Write(content)
	filename := file.Name()
	defer func() {
		_ = file.Close()
		_ = os.Remove(filename)
	}()
	rio.Startup()
	defer rio.Shutdown()

	ctx := rio.Background()

	ln, lnErr := rio.Listen(
		"tcp", ":9000",
		rio.WithParallelAcceptors(10),
	)
	if lnErr != nil {
		t.Error(lnErr)
		return
	}

	lwg := new(sync.WaitGroup)
	lwg.Add(1)
	swg := new(sync.WaitGroup)
	ln.OnAccept(func(ctx context.Context, conn rio.Connection, err error) {
		if err != nil {
			if rio.IsShutdown(err) || async.IsCanceled(err) {
				t.Log("srv accept closed")
			} else {
				t.Error("srv accept:", err)
			}
			lwg.Done()
			return
		}

		t.Log("srv accept:", conn.RemoteAddr(), err)

		swg.Add(1)
		conn.Read().OnComplete(func(ctx context.Context, in transport.Inbound, err error) {
			defer swg.Done()
			if err != nil {
				t.Error("srv read:", err)
				_ = conn.Close()
				return
			}
			n := in.Len()
			rb, _ := in.Next(n)
			t.Log("srv read:", n, bytes.Equal(rb, content))
			_ = conn.Close()
		})
	})

	cwg := new(sync.WaitGroup)
	cwg.Add(1)
	rio.Dial(ctx, "tcp", "127.0.0.1:9000").OnComplete(func(ctx context.Context, conn rio.Connection, err error) {
		if err != nil {
			t.Error("cli dial:", err)
			cwg.Done()
			return
		}
		tcpConn, tcpOk := conn.(rio.TCPConnection)
		if !tcpOk {
			t.Error("conn is not a tcp connection")
			cwg.Done()
			return
		}

		tcpConn.Sendfile(filename).OnComplete(func(ctx context.Context, out int, err error) {
			if err != nil {
				t.Error("cli send:", err)
				cwg.Done()
				return
			}
			t.Log("cli send:", out)
			_ = tcpConn.Close()
			cwg.Done()
		})
	})

	cwg.Wait()

	swg.Wait()
	// close ln
	_ = ln.Close()
	lwg.Wait()
}

func TestConnection_SetReadTimeout(t *testing.T) {
	rio.Startup()
	defer rio.Shutdown()

	timeout := 100 * time.Millisecond

	ln, lnErr := rio.Listen(
		"tcp", ":9000",
		rio.WithParallelAcceptors(1),
		rio.WithFastOpen(1),
		rio.WithDefaultConnReadTimeout(timeout),
	)
	if lnErr != nil {
		t.Error(lnErr)
		return
	}

	lwg := new(sync.WaitGroup)
	lwg.Add(1)
	swg := new(sync.WaitGroup)
	ln.OnAccept(func(ctx context.Context, conn rio.Connection, err error) {
		if err != nil {
			if rio.IsShutdown(err) || async.IsCanceled(err) {
				t.Log("srv accept closed")
			} else {
				t.Error("srv accept:", err)
			}
			lwg.Done()
			return
		}

		t.Log("srv accept:", conn.RemoteAddr(), err)

		swg.Add(1)
		conn.Read().OnComplete(func(ctx context.Context, in transport.Inbound, err error) {
			defer swg.Done()
			if err != nil {
				if rio.IsDeadlineExceeded(err) {
					t.Log("srv deadline exceeded", err)
				} else {
					t.Error("srv read:", err)
				}
				_ = conn.Close()
				return
			}
			n := in.Len()
			b, _ := in.Next(n)
			t.Log("srv read:", n, string(b))
			_ = conn.Close()
		})
	})

	conn, dialErr := net.Dial("tcp", "127.0.0.1:9000")
	if dialErr != nil {
		t.Fatal(dialErr)
		return
	}
	defer conn.Close()

	time.Sleep(timeout * 2)
	n, wErr := conn.Write([]byte("hello word"))
	t.Log("conn write:", n, wErr)

	swg.Wait()
	// close ln
	_ = ln.Close()
	lwg.Wait()

}
