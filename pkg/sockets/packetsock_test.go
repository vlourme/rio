package sockets_test

import (
	"github.com/brickingsoft/rio/pkg/sockets"
	"net"
	"sync"
	"testing"
)

func TestPacket(t *testing.T) {
	conn, lnErr := sockets.ListenPacket("udp", ":9000", sockets.Options{})
	if lnErr != nil {
		t.Error(lnErr)
		return
	}
	defer func(conn sockets.PacketConnection) {
		closeErr := conn.Close()
		if closeErr != nil {
			t.Error(closeErr)
		}
	}(conn)
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func(conn sockets.PacketConnection) {
		p := make([]byte, 1024)
		conn.ReadFrom(p, func(n int, addr net.Addr, err error) {
			t.Log("srv read:", n, addr, err, string(p[:n]))
			go func(conn sockets.PacketConnection) {
				conn.WriteTo(p[:n], addr, func(n int, err error) {
					t.Log("srv write:", n, err)
					wg.Done()
				})
			}(conn)
		})
	}(conn)

	wg.Add(1)
	sockets.Dial("udp", ":9000", sockets.Options{}, func(conn sockets.Connection, err error) {
		if err != nil {
			t.Error(err)
			return
		}
		go func(conn sockets.Connection) {
			conn.Write([]byte("hello world"), func(n int, wErr error) {
				t.Log("cli write:", n, wErr)
				go func(conn sockets.Connection) {
					p := make([]byte, 1024)
					conn.Read(p, func(n int, rErr error) {
						t.Log("cli read:", n, string(p[:n]), rErr)
						wg.Done()
					})
				}(conn)
			})
		}(conn)
	})
	wg.Wait()
}

func TestPacket_Msg(t *testing.T) {
	conn, lnErr := sockets.ListenPacket("udp", ":9000", sockets.Options{})
	if lnErr != nil {
		t.Error(lnErr)
		return
	}
	defer func(conn sockets.PacketConnection) {
		closeErr := conn.Close()
		if closeErr != nil {
			t.Error(closeErr)
		}
	}(conn)
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func(conn sockets.PacketConnection) {
		p := make([]byte, 1024)
		oob := make([]byte, 1024)
		conn.ReadMsg(p, oob, func(n int, oobn int, flags int, addr net.Addr, err error) {
			t.Log("srv read:", n, oobn, flags, addr, err, string(p[:n]), string(oob[:oobn]))
			go func(conn sockets.PacketConnection) {
				conn.WriteMsg(p[:n], oob[:oobn], addr, func(n int, oobn int, err error) {
					t.Log("srv write:", n, oobn, err)
					wg.Done()
				})
			}(conn)
		})
	}(conn)

	addr := &net.UDPAddr{
		IP:   net.ParseIP("127.0.0.1"),
		Port: 9000,
	}

	wg.Add(1)
	sockets.Dial("udp", ":9000", sockets.Options{}, func(conn sockets.Connection, err error) {
		if err != nil {
			t.Error(err)
			return
		}
		go func(pc sockets.PacketConnection) {
			pc.WriteMsg([]byte("hello world"), []byte("oob"), addr, func(n int, oobn int, err error) {
				t.Log("cli write:", n, oobn, err)
				go func(pc sockets.PacketConnection) {
					p := make([]byte, 1024)
					oob := make([]byte, 1024)
					pc.ReadMsg(p, oob, func(n int, oobn int, flags int, rAddr net.Addr, err error) {
						t.Log("cli read:", n, string(p[:n]), oobn, string(oob[:oobn]), flags, rAddr, err)
						wg.Done()
					})
				}(pc)
			})
		}(conn.(sockets.PacketConnection))
	})
	wg.Wait()
}
