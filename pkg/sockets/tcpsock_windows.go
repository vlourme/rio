//go:build windows

package sockets

import (
	"golang.org/x/sys/windows"
	"net"
)

func ListenTCP() (ln Listener, err error) {

	return
}

func DialTCP() (conn Connection, err error) {

	return
}

type tcpListener struct {
	iocp   windows.Handle
	fd     windows.Handle
	addr   net.Addr
	family int
	sotype int
	net    string
}

func (ln *tcpListener) Addr() (addr net.Addr) {
	addr = ln.addr
	return
}

func (ln *tcpListener) Accept() (err error) {

	return
}

func (ln *tcpListener) Close() (err error) {
	return
}

func (ln *tcpListener) polling() {

}

type tcpConnection struct {
	iocp       windows.Handle
	ln         windows.Handle
	fd         windows.Handle
	localAddr  net.Addr
	remoteAddr net.Addr
	family     int
	sotype     int
	net        string
}
