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
			wg.Done()
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
	closeErr := cli.Close()
	if closeErr != nil {
		t.Error(closeErr)
		return
	}
	wg.Wait()
}
