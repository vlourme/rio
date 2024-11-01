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

type connection struct {
	cphandle   windows.Handle
	fd         windows.Handle
	localAddr  net.Addr
	remoteAddr net.Addr
	net        string
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

func (conn *connection) Close(handler CloseHandler) (err error) {
	// todo async or sync
	//windows.PostQueuedCompletionStatus()
	_ = windows.Shutdown(conn.fd, 2)
	_ = windows.Closesocket(conn.fd)
	_ = windows.CloseHandle(conn.cphandle)
	return
}
