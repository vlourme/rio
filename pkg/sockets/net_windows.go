//go:build windows

package sockets

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"net"
	"net/netip"
	"os"
)

type Fd struct {
	SysFd         windows.Handle
	IsStream      bool
	ZeroReadIsEOF bool
}

type operation struct {
	// Used by IOCP interface, it must be first field
	// of the struct, as our code rely on it.
	overlapped windows.Overlapped
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
	// fields used only by net callback
	acceptHandler              AcceptHandler
	readHandler                ReadHandler
	writeHandler               WriteHandler
	readFromHandler            ReadFromHandler
	readFromUDPHandler         ReadFromUDPHandler
	readFromUDPAddrPortHandler ReadFromUDPAddrPortHandler
	readMsgUDPHandler          ReadMsgUDPHandler
	readMsgUDPAddrPortHandler  ReadMsgUDPAddrPortHandler
	writeMsgHandler            WriteMsgHandler
	readFromUnixHandler        ReadFromUnixHandler
	readMsgUnixHandler         ReadMsgUnixHandler
	unixAcceptHandler          UnixAcceptHandler
	closeHandler               CloseHandler
}

func (op *operation) failed(cause error) {
	switch op.mode {
	case accept:
		op.acceptHandler(nil, cause)
		break
	case read:
		op.readHandler(0, cause)
		break
	case write:
		op.writeHandler(0, cause)
		break
	case readFrom:
		op.readFromHandler(0, nil, cause)
		break
	case readFromUDP:
		op.readFromUDPHandler(0, nil, cause)
		break
	case readFromUDPAddrPort:
		op.readFromUDPAddrPortHandler(0, netip.AddrPort{}, cause)
		break
	case readMsgUDP:
		op.readMsgUDPHandler(0, 0, 0, nil, cause)
		break
	case writeMsg:
		op.writeMsgHandler(0, 0, nil)
		break
	case readFromUnix:
		op.readFromUnixHandler(0, nil, cause)
		break
	case readMsgUnix:
		op.readMsgUnixHandler(0, nil, 0, nil, cause)
		break
	case unixAccept:
		op.unixAcceptHandler(nil, cause)
		break
	case disconnect:
		op.closeHandler(cause)
		break
	case exit:
		break
	default:
		break
	}
}

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
	iocp       windows.Handle
	ln         windows.Handle
	fd         windows.Handle
	localAddr  net.Addr
	remoteAddr net.Addr
	family     int
	sotype     int
	net        string
}
