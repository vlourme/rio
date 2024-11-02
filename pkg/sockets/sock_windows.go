//go:build windows

package sockets

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"net"
	"os"
	"time"
)

func wrapSyscallError(name string, err error) error {
	var errno windows.Errno
	if errors.As(err, &errno) {
		err = os.NewSyscallError(name, err)
	}
	return err
}

func wsaStartup() (windows.WSAData, error) {
	var d windows.WSAData
	startupErr := windows.WSAStartup(uint32(0x202), &d)
	if startupErr != nil {
		fmt.Printf("Error starting WSAStartup: %v", startupErr)
		return d, startupErr
	}
	return d, nil
}

func wsaCleanup() {
	_ = windows.WSACleanup()
}

func newConnection(network string, fd windows.Handle) (conn *connection) {
	conn = &connection{net: network, fd: fd}
	conn.rop.conn = conn
	conn.wop.conn = conn
	return
}

type connection struct {
	cphandle   windows.Handle
	fd         windows.Handle
	localAddr  net.Addr
	remoteAddr net.Addr
	net        string
	rop        operation
	wop        operation
}

func (conn *connection) LocalAddr() (addr net.Addr) {
	addr = conn.localAddr
	return
}

func (conn *connection) RemoteAddr() (addr net.Addr) {
	addr = conn.remoteAddr
	return
}

func (conn *connection) SetDeadline(t time.Time) (err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *connection) SetReadDeadline(t time.Time) (err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *connection) SetWriteDeadline(t time.Time) (err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *connection) SetReadBuffer(bytes int) (err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *connection) SetWriteBuffer(bytes int) (err error) {
	//TODO implement me
	panic("implement me")
}

func (conn *connection) Close() (err error) {
	_ = windows.Shutdown(conn.fd, 2)
	err = windows.Closesocket(conn.fd)
	if err != nil {
		err = &net.OpError{
			Op:     "close",
			Net:    conn.net,
			Source: conn.localAddr,
			Addr:   conn.remoteAddr,
			Err:    err,
		}
	}
	_ = windows.CloseHandle(conn.cphandle)
	return
}
