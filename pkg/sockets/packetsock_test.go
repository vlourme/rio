package sockets_test

import (
	"github.com/brickingsoft/rio/pkg/sockets"
	"net"
	"sync"
	"testing"
)

func TestListenPacket(t *testing.T) {
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
				conn.WriteTo(p, addr, func(n int, err error) {
					t.Log("srv write:", n, err)
					wg.Done()
				})
			}(conn)
		})
	}(conn)

	cli, dialErr := net.DialUDP("udp", nil, &net.UDPAddr{
		IP:   net.IPv4(0, 0, 0, 0),
		Port: 9000,
	})
	if dialErr != nil {
		t.Error(dialErr)
		return
	}
	n, wErr := cli.Write([]byte("hello world"))
	if wErr != nil {
		t.Error(wErr)
	}
	t.Log("cli write:", n)
	p := make([]byte, 1024)
	rn, rErr := cli.Read(p)
	if rErr != nil {
		t.Error(rErr)
	}
	t.Log("cli read:", rn, string(p[:rn]))
	closeErr := cli.Close()
	if closeErr != nil {
		t.Error(closeErr)
		return
	}
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
		IP:   net.IPv4(0, 0, 0, 0),
		Port: 9000,
	}
	cli, dialErr := net.DialUDP("udp", nil, addr)
	if dialErr != nil {
		t.Error(dialErr)
		return
	}
	n, oobn, wErr := cli.WriteMsgUDP([]byte("hello world"), []byte("oob"), nil)
	if wErr != nil {
		t.Error(wErr)
	}
	t.Log("cli write:", n, oobn)
	p := make([]byte, 1024)
	oob := make([]byte, 1024)
	rn, roobn, flags, rAddr, rErr := cli.ReadMsgUDP(p, oob)
	if rErr != nil {
		t.Error(rErr)
	}
	t.Log("cli read:", rn, string(p[:rn]), roobn, string(oob[:roobn]), flags, rAddr)
	closeErr := cli.Close()
	if closeErr != nil {
		t.Error(closeErr)
		return
	}
	wg.Wait()
}
