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

func listenUDP(network string, family int, addr *net.UDPAddr, ipv6only bool, proto int) (conn PacketConnection, err error) {
	conn, err = newUDPConnection(network, family, addr, ipv6only, proto)
	return
}

func newUDPConnection(network string, family int, addr *net.UDPAddr, ipv6only bool, proto int) (uc *udpConnection, err error) {
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
	// udp
	uc = &udpConnection{
		connection: *conn,
	}
	return
}

type udpConnection struct {
	connection
}

func (conn *udpConnection) ReadFrom(p []byte, handler ReadFromHandler) {
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

func (conn *udpConnection) WriteTo(p []byte, addr net.Addr, handler WriteHandler) {
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

func (conn *udpConnection) ReadMsg(p []byte, oob []byte, handler ReadMsgHandler) {
	pLen := len(p)
	if pLen == 0 {
		handler(0, 0, 0, nil, ErrEmptyPacket)
		return
	}
	if pLen > maxRW {
		p = p[:maxRW]
	}
	conn.rop.mode = readMsgUDP
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

func (conn *udpConnection) WriteMsg(p []byte, oob []byte, addr net.Addr, handler WriteMsgHandler) {
	//TODO implement me
	panic("implement me")
}
