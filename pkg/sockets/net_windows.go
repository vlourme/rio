//go:build windows

package sockets

import (
	"golang.org/x/sys/windows"
	"net"
)

type Fd struct {
	SysFd         windows.Handle
	IsStream      bool
	ZeroReadIsEOF bool
}

type operation struct {
	// Used by IOCP interface, it must be first field
	// of the struct, as our code rely on it.
	Overlapped windows.Overlapped
	mode       OperationMode

	// fields used only by net package
	fd     *Fd
	buf    windows.WSABuf
	msg    windows.WSAMsg
	sa     windows.Sockaddr
	rsa    *windows.RawSockaddrAny
	rsan   int32
	handle windows.Handle
	flags  uint32
	qty    uint32
	bufs   []windows.WSABuf
}

type connection struct {
	iocp       windows.Handle
	ln         windows.Handle
	fd         windows.Handle
	localAddr  net.Addr
	remoteAddr net.Addr
	family     int
	sotype     int
	net        string
}
