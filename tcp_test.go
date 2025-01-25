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
	_ = rio.Startup()
	defer func() {
		_ = rio.ShutdownGracefully()
	}()

	ctx := context.Background()

	ln, lnErr := rio.Listen(
		ctx,
		"tcp", "127.0.0.1:9000",
		rio.WithParallelAcceptors(1),
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
	ln.Accept().OnComplete(func(ctx context.Context, conn rio.Connection, err error) {
		if err != nil {
			t.Log("accepted:", rio.IsClosed(err), err, ctx.Err())
			lwg.Done()
			return
		}

		var addr net.Addr
		if conn != nil {
			addr = conn.RemoteAddr()
		}
		t.Log("accepted:", addr, err, ctx.Err())
		if conn != nil {
			conn.Close().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
				if cause != nil {
					t.Error("srv close conn err:", cause)
				}
				awg.Done()
			})
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

	lwg.Add(1)
	ln.Close().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
		if cause != nil {
			t.Error("ln close close err:", cause)
		}
		lwg.Done()
	})
	lwg.Wait()
}

func TestTCP(t *testing.T) {
	_ = rio.Startup()
	defer func() {
		_ = rio.ShutdownGracefully()
	}()

	ctx := context.Background()

	ln, lnErr := rio.Listen(ctx,
		"tcp", ":9000",
		rio.WithParallelAcceptors(1),
		rio.WithPromiseMakeOptions(async.WithDirectMode()),
		rio.WithFastOpen(1),
	)
	if lnErr != nil {
		t.Error(lnErr)
		return
	}

	//timeout := time.Second * 1
	timeout := time.Second * 0

	lwg := new(sync.WaitGroup)
	lwg.Add(1)
	swg := new(sync.WaitGroup)
	swg.Add(1)
	ln.Accept().OnComplete(func(ctx context.Context, conn rio.Connection, err error) {
		if err != nil {
			if rio.IsClosed(err) || async.IsEOF(err) {
				t.Log("srv accept closed")
			} else {
				t.Error("srv accept:", err)
			}
			lwg.Done()
			return
		}

		t.Log("srv accept:", conn.RemoteAddr(), err)
		if timeout > 0 {
			conn.SetReadTimeout(timeout)
		}
		conn.Read().OnComplete(func(ctx context.Context, in transport.Inbound, err error) {
			if err != nil {
				t.Error("srv read:", err)
				conn.Close().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
					swg.Done()
				})
				return
			}
			n := in.Received()
			p, _ := in.Reader().Next(n)
			t.Log("srv read:", n, string(p))
			conn.Write(p).OnComplete(func(ctx context.Context, out int, err error) {
				if err != nil {
					t.Error("srv write:", err)
					conn.Close().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
						t.Log("srv close:", cause)
						swg.Done()
					})
					return
				}
				t.Log("srv write:", out)
				conn.Close().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
					t.Log("srv close:", cause)
					swg.Done()
				})
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
		if timeout > 0 {
			time.Sleep(timeout)
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
				t.Log("cli read:", in.Received(), string(in.Reader().Peek(in.Received())))
				conn.Close().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
					cwg.Done()
					t.Log("cli close:", err)
				})
			})
		})
	})

	cwg.Wait()
	swg.Wait()

	lwg.Add(1)
	ln.Close().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
		t.Log("ln close:", cause)
		lwg.Done()
	})
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
	_ = rio.Startup()
	defer func() {
		_ = rio.ShutdownGracefully()
	}()

	ctx := context.Background()

	ln, lnErr := rio.Listen(ctx,
		"tcp", ":9000",
		rio.WithParallelAcceptors(10),
		rio.WithPromiseMakeOptions(async.WithDirectMode()),
	)
	if lnErr != nil {
		t.Error(lnErr)
		return
	}

	lwg := new(sync.WaitGroup)
	lwg.Add(1)
	swg := new(sync.WaitGroup)
	ln.Accept().OnComplete(func(ctx context.Context, conn rio.Connection, err error) {
		if err != nil {
			if rio.IsClosed(err) || async.IsEOF(err) {
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
				conn.Close().OnComplete(func(ctx context.Context, entry async.Void, cause error) {})
				return
			}
			n := in.Received()
			rb, _ := in.Reader().Next(n)
			t.Log("srv read:", n, bytes.Equal(rb, content))

			swg.Add(1)
			conn.Close().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
				defer swg.Done()
				t.Log("srv close:", cause)
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
			conn.Close().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
				cwg.Done()
				t.Log("cli close:", err)
			})
		})
	})

	cwg.Wait()

	time.Sleep(50 * time.Millisecond)

	swg.Wait()
	// close ln
	lwg.Add(1)
	ln.Close().OnComplete(func(ctx context.Context, entry async.Void, cause error) {
		t.Log("ln close:", cause)
		lwg.Done()
	})
	lwg.Wait()
}
