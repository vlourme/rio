//go:build windows

package sockets

import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"net"
	"syscall"
	"unsafe"
)

func newPacketConnection(network string, family int, addr net.Addr, ipv6only bool, proto int) (pc PacketConnection, err error) {
	// conn
	conn, connErr := newConnection(network, family, windows.SOCK_DGRAM, proto, ipv6only)
	if connErr != nil {
		err = connErr
		return
	}
	// bind
	lsa := addrToSockaddr(family, addr)
	bindErr := windows.Bind(conn.fd, lsa)
	if bindErr != nil {
		err = bindErr
		_ = windows.Closesocket(conn.fd)
		return
	}
	// CreateIoCompletionPort
	cphandle, createErr := createSubIoCompletionPort(conn.fd)
	if createErr != nil {
		_ = windows.Closesocket(conn.fd)
		err = createErr
		return
	}
	conn.cphandle = cphandle
	// as packet conn
	pc = conn
	return
}

func (conn *connection) ReadFrom(p []byte, handler ReadFromHandler) {
	pLen := len(p)
	if pLen == 0 {
		handler(0, nil, ErrEmptyPacket)
		return
	}
	if pLen > maxRW {
		p = p[:maxRW]
	}
	conn.rop.mode = readFrom
	conn.rop.buf.Buf = &p[0]
	conn.rop.buf.Len = uint32(pLen)
	if conn.rop.rsa == nil {
		conn.rop.rsa = new(windows.RawSockaddrAny)
	}
	conn.rop.rsan = int32(unsafe.Sizeof(*conn.rop.rsa))
	conn.rop.readFromHandler = handler
	err := windows.WSARecvFrom(conn.fd, &conn.rop.buf, 1, &conn.rop.qty, &conn.rop.flags, conn.rop.rsa, &conn.rop.rsan, &conn.rop.overlapped, nil)
	if err != nil && !errors.Is(windows.ERROR_IO_PENDING, err) {
		if errors.Is(windows.ERROR_TIMEOUT, err) {
			err = context.DeadlineExceeded
		}
		handler(0, nil, wrapSyscallError("WSARecvFrom", err))
		conn.rop.readFromHandler = nil
	}
	return
}

func (op *operation) completeReadFrom(qty int, err error) {
	sockaddr, sockaddrErr := op.rsa.Sockaddr()
	if sockaddrErr != nil {
		op.readFromHandler(qty, nil, sockaddrErr)
		op.readFromHandler = nil
		return
	}
	addr := sockaddrToAddr(op.conn.net, sockaddr)
	if err != nil {
		op.readFromHandler(qty, addr, &net.OpError{
			Op:     op.mode.String(),
			Net:    op.conn.net,
			Source: op.conn.localAddr,
			Addr:   addr,
			Err:    err,
		})
		op.readFromHandler = nil
		return
	}
	op.readFromHandler(qty, addr, op.eofError(qty, err))
	op.readFromHandler = nil
	return
}

func (conn *connection) WriteTo(p []byte, addr net.Addr, handler WriteHandler) {
	pLen := len(p)
	if pLen == 0 {
		handler(0, ErrEmptyPacket)
		return
	} else if pLen > maxRW {
		p = p[:maxRW]
		pLen = maxRW
	}
	conn.wop.mode = writeTo
	conn.wop.buf.Buf = &p[0]
	conn.wop.buf.Len = uint32(pLen)
	conn.wop.sa = addrToSockaddr(conn.family, addr)
	conn.wop.writeHandler = handler
	err := windows.WSASendto(conn.fd, &conn.wop.buf, 1, &conn.wop.qty, conn.wop.flags, conn.wop.sa, &conn.wop.overlapped, nil)
	if err != nil && !errors.Is(windows.ERROR_IO_PENDING, err) {
		if errors.Is(windows.ERROR_TIMEOUT, err) {
			err = context.DeadlineExceeded
		}
		handler(0, wrapSyscallError("WSASend", err))
		conn.wop.writeHandler = nil
	}
	return
}

func (op *operation) completeWriteTo(qty int, err error) {
	if err != nil {
		op.writeHandler(0, &net.OpError{
			Op:     op.mode.String(),
			Net:    op.conn.net,
			Source: op.conn.localAddr,
			Addr:   sockaddrToAddr(op.conn.net, op.sa),
			Err:    err,
		})
		op.writeHandler = nil
		return
	}
	op.writeHandler(qty, nil)
	op.writeHandler = nil
	return
}

func (conn *connection) ReadMsg(p []byte, oob []byte, handler ReadMsgHandler) {
	pLen := len(p)
	if pLen == 0 {
		handler(0, 0, 0, nil, ErrEmptyPacket)
		return
	}
	if pLen > maxRW {
		p = p[:maxRW]
	}
	conn.rop.mode = readMsg
	conn.rop.InitMsg(p, oob)
	conn.rop.msg.Name = (*syscall.RawSockaddrAny)(unsafe.Pointer(conn.rop.rsa))
	conn.rop.msg.Namelen = int32(unsafe.Sizeof(*conn.rop.rsa))
	conn.rop.msg.Flags = uint32(0)
	if conn.rop.rsa == nil {
		conn.rop.rsa = new(windows.RawSockaddrAny)
	}
	conn.rop.rsan = int32(unsafe.Sizeof(*conn.rop.rsa))
	conn.rop.readMsgHandler = handler
	err := windows.WSARecvMsg(conn.fd, &conn.rop.msg, &conn.rop.qty, &conn.rop.overlapped, nil)
	if err != nil && !errors.Is(windows.ERROR_IO_PENDING, err) {
		if errors.Is(windows.ERROR_TIMEOUT, err) {
			err = context.DeadlineExceeded
		}
		handler(0, 0, 0, nil, wrapSyscallError("WSARecvMsg", err))
		conn.rop.readMsgHandler = nil
	}
	return
}

func (op *operation) completeReadMsg(qty int, err error) {
	// todo
	panic("implement me")
	return
}

func (conn *connection) WriteMsg(p []byte, oob []byte, addr net.Addr, handler WriteMsgHandler) {
	//TODO implement me
	panic("implement me")
}

func (op *operation) completeWriteMsg(qty int, err error) {
	// todo
	panic("implement me")
	return
}
