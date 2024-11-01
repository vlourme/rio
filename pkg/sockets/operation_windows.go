//go:build windows

package sockets

import (
	"golang.org/x/sys/windows"
	"io"
	"net"
	"net/netip"
	"os"
	"unsafe"
)

type operation struct {
	// Used by IOCP interface, it must be first field
	// of the struct, as our code rely on it.
	overlapped windows.Overlapped
	mode       OperationMode
	// fields used only by net package
	conn   connection
	buf    windows.WSABuf
	msg    windows.WSAMsg
	sa     windows.Sockaddr
	rsa    *windows.RawSockaddrAny
	rsan   int32
	iocp   windows.Handle
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

func (op *operation) handleAccept() {
	conn := op.conn
	// set SO_UPDATE_ACCEPT_CONTEXT
	setAcceptSocketOptErr := windows.Setsockopt(
		conn.fd,
		windows.SOL_SOCKET, windows.SO_UPDATE_ACCEPT_CONTEXT,
		(*byte)(unsafe.Pointer(&op.handle)),
		int32(unsafe.Sizeof(op.handle)),
	)
	if setAcceptSocketOptErr != nil {
		op.acceptHandler(nil, os.NewSyscallError("setsockopt", setAcceptSocketOptErr))
		op.acceptHandler = nil
		return
	}
	// get addr
	lsa, lsaErr := windows.Getsockname(conn.fd)
	if lsaErr != nil {
		op.acceptHandler(nil, os.NewSyscallError("getsockname", lsaErr))
		op.acceptHandler = nil
		return
	}
	la := sockaddrToTCPAddr(lsa)
	conn.localAddr = la
	rsa, rsaErr := windows.Getpeername(op.conn.fd)
	if rsaErr != nil {
		op.acceptHandler(nil, os.NewSyscallError("getsockname", rsaErr))
		op.acceptHandler = nil
		return
	}
	ra := sockaddrToTCPAddr(rsa)
	conn.remoteAddr = ra
	// CreateIoCompletionPort
	cphandle, createErr := windows.CreateIoCompletionPort(op.conn.fd, op.iocp, 0, 0)
	if createErr != nil {
		op.acceptHandler(nil, os.NewSyscallError("createIoCompletionPort", createErr))
		op.acceptHandler = nil
		return
	}
	conn.cphandle = cphandle
	// callback
	tcpConn := tcpConnection{
		connection: conn,
	}
	op.acceptHandler(&tcpConn, nil)
	op.acceptHandler = nil
}

func (op *operation) handleRead() {
	if op.qty == 0 {
		op.readHandler(0, io.EOF)
	} else {
		op.readHandler(int(op.qty), nil)
	}
	op.readHandler = nil
	return
}

func (op *operation) handleWrite() {
	op.writeHandler(int(op.qty), nil)
	op.writeHandler = nil
	return
}

func (op *operation) handleDisconnect() {
	op.closeHandler(nil)
	op.closeHandler = nil
	return
}

func (op *operation) failed(cause error) {
	switch op.mode {
	case accept:
		op.acceptHandler(nil, cause)
		break
	case read:
		op.readHandler(0, &net.OpError{
			Op:     "read",
			Net:    op.conn.net,
			Source: op.conn.localAddr,
			Addr:   op.conn.remoteAddr,
			Err:    cause,
		})
		break
	case write:
		op.writeHandler(0, &net.OpError{
			Op:     "write",
			Net:    op.conn.net,
			Source: op.conn.localAddr,
			Addr:   op.conn.remoteAddr,
			Err:    cause,
		})
		break
	case readFrom:
		op.readFromHandler(0, nil, &net.OpError{
			Op:     "readFrom",
			Net:    op.conn.net,
			Source: op.conn.localAddr,
			Addr:   op.conn.remoteAddr,
			Err:    cause,
		})
		break
	case readFromUDP:
		op.readFromUDPHandler(0, nil, &net.OpError{
			Op:     "readFromUDP",
			Net:    op.conn.net,
			Source: op.conn.localAddr,
			Addr:   op.conn.remoteAddr,
			Err:    cause,
		})
		break
	case readFromUDPAddrPort:
		op.readFromUDPAddrPortHandler(0, netip.AddrPort{}, &net.OpError{
			Op:     "readFromUDPAddrPort",
			Net:    op.conn.net,
			Source: op.conn.localAddr,
			Addr:   op.conn.remoteAddr,
			Err:    cause,
		})
		break
	case readMsgUDP:
		op.readMsgUDPHandler(0, 0, 0, nil, &net.OpError{
			Op:     "readMsgUDP",
			Net:    op.conn.net,
			Source: op.conn.localAddr,
			Addr:   op.conn.remoteAddr,
			Err:    cause,
		})
		break
	case writeMsg:
		op.writeMsgHandler(0, 0, &net.OpError{
			Op:     "writeMsg",
			Net:    op.conn.net,
			Source: op.conn.localAddr,
			Addr:   op.conn.remoteAddr,
			Err:    cause,
		})
		break
	case readFromUnix:
		op.readFromUnixHandler(0, nil, &net.OpError{
			Op:     "readFromUnix",
			Net:    op.conn.net,
			Source: op.conn.localAddr,
			Addr:   op.conn.remoteAddr,
			Err:    cause,
		})
		break
	case readMsgUnix:
		op.readMsgUnixHandler(0, nil, 0, nil, &net.OpError{
			Op:     "readMsgUnix",
			Net:    op.conn.net,
			Source: op.conn.localAddr,
			Addr:   op.conn.remoteAddr,
			Err:    cause,
		})
		break
	case unixAccept:
		op.unixAcceptHandler(nil, cause)
		break
	case disconnect:
		op.closeHandler(&net.OpError{
			Op:     "close",
			Net:    op.conn.net,
			Source: op.conn.localAddr,
			Addr:   op.conn.remoteAddr,
			Err:    cause,
		})
		break
	default:
		break
	}
}
