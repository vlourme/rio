//go:build windows

package sockets

import (
	"golang.org/x/sys/windows"
	"io"
	"net"
	"os"
	"unsafe"
)

type operation struct {
	// Used by IOCP interface, it must be first field
	// of the struct, as our code rely on it.
	overlapped windows.Overlapped
	mode       OperationMode
	// fields used only by net package
	conn   *connection
	buf    windows.WSABuf
	msg    windows.WSAMsg
	sa     windows.Sockaddr
	rsa    *windows.RawSockaddrAny
	rsan   int32
	iocp   windows.Handle
	handle windows.Handle
	flags  uint32
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
}

func (op *operation) complete(qty int, err error) {
	switch op.mode {
	case accept:
		op.completeAccept(qty, err)
		break
	case unixAccept:
		op.completeUnixAccept(qty, err)
		break
	case read:
		op.completeRead(qty, err)
		break
	case write:
		op.completeWrite(qty, err)
		break
	case readFrom:
		op.completeReadFrom(qty, err)
		break
	case readFromUDP:
		op.completeReadFromUDP(qty, err)
		break
	case readFromUDPAddrPort:
		op.completeReadFromUDPAddrPort(qty, err)
		break
	case readMsgUDP:
		op.completeReadMsgUDP(qty, err)
		break
	case writeMsg:
		op.completeWriteMsg(qty, err)
		break
	case readFromUnix:
		op.completeReadFromUnix(qty, err)
		break
	case readMsgUnix:
		op.completeReadMsgUnix(qty, err)
		break
	default:
		break
	}
}

func (op *operation) completeAccept(_ int, err error) {
	if err != nil {
		op.acceptHandler(nil, os.NewSyscallError("AcceptEx", err))
		op.acceptHandler = nil
		op.handle = 0
		op.iocp = 0
		return
	}
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
		op.handle = 0
		op.iocp = 0
		return
	}
	// get addr
	lsa, lsaErr := windows.Getsockname(conn.fd)
	if lsaErr != nil {
		op.acceptHandler(nil, os.NewSyscallError("getsockname", lsaErr))
		op.acceptHandler = nil
		op.handle = 0
		op.iocp = 0
		return
	}
	la := sockaddrToTCPAddr(lsa)
	conn.localAddr = la
	rsa, rsaErr := windows.Getpeername(op.conn.fd)
	if rsaErr != nil {
		op.acceptHandler(nil, os.NewSyscallError("getsockname", rsaErr))
		op.acceptHandler = nil
		op.handle = 0
		op.iocp = 0
		return
	}
	ra := sockaddrToTCPAddr(rsa)
	conn.remoteAddr = ra
	// CreateIoCompletionPort
	cphandle, createErr := windows.CreateIoCompletionPort(op.conn.fd, op.iocp, 0, 0)
	if createErr != nil {
		op.acceptHandler(nil, os.NewSyscallError("createIoCompletionPort", createErr))
		op.acceptHandler = nil
		op.handle = 0
		op.iocp = 0
		return
	}
	conn.cphandle = cphandle
	// callback
	tcpConn := tcpConnection{
		connection: *conn,
	}
	op.acceptHandler(&tcpConn, nil)
	op.acceptHandler = nil
	op.handle = 0
	op.iocp = 0
	return
}

func (op *operation) completeRead(qty int, err error) {
	if err != nil {
		op.readHandler(0, &net.OpError{
			Op:     op.mode.String(),
			Net:    op.conn.net,
			Source: op.conn.localAddr,
			Addr:   op.conn.remoteAddr,
			Err:    err,
		})
		op.readHandler = nil
		return
	}
	if qty == 0 {
		op.readHandler(0, io.EOF)
	} else {
		op.readHandler(qty, nil)
	}
	op.readHandler = nil
	return
}

func (op *operation) completeWrite(qty int, err error) {
	if err != nil {
		op.writeHandler(0, &net.OpError{
			Op:     op.mode.String(),
			Net:    op.conn.net,
			Source: op.conn.localAddr,
			Addr:   op.conn.remoteAddr,
			Err:    err,
		})
		op.writeHandler = nil
		return
	}
	op.writeHandler(qty, nil)
	op.writeHandler = nil
	return
}

func (op *operation) completeReadFrom(qty int, err error) {
	// todo
	panic("implement me")
	return
}

func (op *operation) completeReadFromUDP(qty int, err error) {
	// todo
	panic("implement me")
	return
}

func (op *operation) completeReadFromUDPAddrPort(qty int, err error) {
	// todo
	panic("implement me")
	return
}

func (op *operation) completeReadMsgUDP(qty int, err error) {
	// todo
	panic("implement me")
	return
}

func (op *operation) completeReadMsgUDPAddrPort(qty int, err error) {
	// todo
	panic("implement me")
	return
}

func (op *operation) completeWriteMsg(qty int, err error) {
	// todo
	panic("implement me")
	return
}

func (op *operation) completeReadFromUnix(qty int, err error) {
	// todo
	panic("implement me")
	return
}

func (op *operation) completeReadMsgUnix(qty int, err error) {
	// todo
	panic("implement me")
	return
}

func (op *operation) completeUnixAccept(_ int, err error) {
	// todo
	panic("implement me")
	return
}
