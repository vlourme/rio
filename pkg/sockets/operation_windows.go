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
	qty    uint32
	// fields used only by net callback
	tcpAcceptHandler           TCPAcceptHandler
	unixAcceptHandler          UnixAcceptHandler
	listenPacketHandler        ListenPacketHandler // todo use listen udp handler
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
}

func (op *operation) complete(qty int, err error) {
	switch op.mode {
	case tcpAccept:
		op.completeTCPAccept(qty, err)
		break
	case udpAccept:
		op.completeUDPAccept(qty, err)
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
	op.reset()
}

func (op *operation) reset() {
	op.overlapped.Offset = 0
	op.overlapped.OffsetHigh = 0
	op.overlapped.Internal = 0
	op.overlapped.InternalHigh = 0
	op.overlapped.HEvent = 0
	op.mode = 0
}

func (op *operation) eofError(qty int, err error) error {
	if qty == 0 && err == nil && op.conn.zeroReadIsEOF {
		return io.EOF
	}
	return err
}

func (op *operation) completeTCPAccept(_ int, err error) {
	if err != nil {
		op.tcpAcceptHandler(nil, os.NewSyscallError("AcceptEx", err))
		op.tcpAcceptHandler = nil
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
		op.tcpAcceptHandler(nil, os.NewSyscallError("setsockopt", setAcceptSocketOptErr))
		op.tcpAcceptHandler = nil
		return
	}
	// get addr
	lsa, lsaErr := windows.Getsockname(conn.fd)
	if lsaErr != nil {
		op.tcpAcceptHandler(nil, os.NewSyscallError("getsockname", lsaErr))
		op.tcpAcceptHandler = nil
		return
	}
	la := sockaddrToTCPAddr(lsa)
	conn.localAddr = la
	rsa, rsaErr := windows.Getpeername(op.conn.fd)
	if rsaErr != nil {
		op.tcpAcceptHandler(nil, os.NewSyscallError("getsockname", rsaErr))
		op.tcpAcceptHandler = nil
		return
	}
	ra := sockaddrToTCPAddr(rsa)
	conn.remoteAddr = ra
	// CreateIoCompletionPort
	cphandle, createErr := windows.CreateIoCompletionPort(op.conn.fd, op.iocp, key, 0)
	if createErr != nil {
		op.tcpAcceptHandler(nil, os.NewSyscallError("createIoCompletionPort", createErr))
		op.tcpAcceptHandler = nil
		return
	}
	conn.cphandle = cphandle
	// callback
	tcpConn := tcpConnection{
		connection: *conn,
	}
	op.tcpAcceptHandler(&tcpConn, nil)
	op.tcpAcceptHandler = nil
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
	op.readHandler(qty, op.eofError(qty, err))
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

func (op *operation) completeUDPAccept(_ int, err error) {
	// todo
	panic("implement me")
	return
}

func (op *operation) completeReadFrom(qty int, err error) {
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
	sockaddr, _ := op.rsa.Sockaddr()
	var addr net.Addr
	if op.conn.net == "unix" {
		addr = sockaddrToUnixAddr(sockaddr)
	} else {
		addr = sockaddrToUDPAddr(sockaddr)
	}
	op.readFromHandler(qty, addr, op.eofError(qty, err))
	op.readFromHandler = nil
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
