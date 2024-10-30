//go:build windows

package sockets

import (
	"golang.org/x/sys/windows"
	"net"
)

type TCPOperationResultHandler func(mode OperationMode, conn *TCPConnection, p []byte, n int, cause error)

func Listen() (ln *TCPListener, err error) {

	return
}

type TCPListener struct {
	iocp   windows.Handle
	fd     windows.Handle
	addr   net.Addr
	family int
	sotype int
	net    string
}

func (ln *TCPListener) Addr() (addr net.Addr) {
	addr = ln.addr
	return
}

func (ln *TCPListener) Accept(handler TCPOperationResultHandler) (err error) {

	return
}

func (ln *TCPListener) Close() (err error) {
	return
}

func (ln *TCPListener) loop() {

}

type TCPConnection struct {
	iocp       windows.Handle
	ln         windows.Handle
	fd         windows.Handle
	localAddr  net.Addr
	remoteAddr net.Addr
	family     int
	sotype     int
	net        string
}
