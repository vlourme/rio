//go:build windows

package sockets

import (
	"errors"
	"golang.org/x/sys/windows"
	"net"
	"syscall"
	"unsafe"
)

func newPacketConnection(network string, family int, sotype int, laddr net.Addr, raddr net.Addr, ipv6only bool, proto int) (pc PacketConnection, err error) {
	// conn
	conn, connErr := newConnection(network, family, sotype, proto, ipv6only)
	if connErr != nil {
		err = connErr
		return
	}
	if laddr != nil {
		lsa := addrToSockaddr(family, laddr)
		bindErr := windows.Bind(conn.fd, lsa)
		if bindErr != nil {
			err = bindErr
			_ = conn.Closesocket()
			return
		}
		conn.localAddr = laddr
	}
	if raddr != nil {
		rsa := addrToSockaddr(family, raddr)
		connectErr := windows.Connect(conn.fd, rsa)
		if connectErr != nil {
			err = wrapSyscallError("connect", connectErr)
			_ = conn.Closesocket()
			return
		}
		conn.remoteAddr = raddr
		lsa, _ := windows.Getsockname(conn.fd)
		conn.localAddr = sockaddrToAddr(network, lsa)
	}

	// CreateIoCompletionPort
	cphandle, createErr := createSubIoCompletionPort(conn.fd)
	if createErr != nil {
		_ = conn.Closesocket()
		err = createErr
		return
	}
	conn.cphandle = cphandle
	// connected
	conn.connected.Store(true)
	// as packet conn
	pc = conn
	return
}

func (conn *connection) ReadFrom(p []byte, handler ReadFromHandler) {
	if !conn.ok() {
		handler(0, nil, wrapSyscallError("WSARecvFrom", syscall.EINVAL))
		return
	}
	pLen := len(p)
	if pLen == 0 {
		handler(0, nil, ErrEmptyPacket)
		return
	}
	if pLen > maxRW {
		p = p[:maxRW]
	}
	conn.rop.mode = readFrom
	conn.rop.InitBuf(p)
	if conn.rop.rsa == nil {
		conn.rop.rsa = new(windows.RawSockaddrAny)
	}
	conn.rop.rsan = int32(unsafe.Sizeof(*conn.rop.rsa))
	conn.rop.readFromHandler = handler
	err := windows.WSARecvFrom(conn.fd, &conn.rop.buf, 1, &conn.rop.qty, &conn.rop.flags, conn.rop.rsa, &conn.rop.rsan, &conn.rop.overlapped, nil)
	if err != nil && !errors.Is(windows.ERROR_IO_PENDING, err) {
		err = &net.OpError{
			Op:     readFrom.String(),
			Net:    conn.net,
			Source: conn.localAddr,
			Addr:   conn.remoteAddr,
			Err:    errors.Join(ErrUnexpectedCompletion, wrapSyscallError("WSARecvFrom", err)),
		}
		handler(0, nil, err)
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
	if !conn.ok() {
		handler(0, wrapSyscallError("WSASend", syscall.EINVAL))
		return
	}
	pLen := len(p)
	if pLen == 0 {
		handler(0, ErrEmptyPacket)
		return
	} else if pLen > maxRW {
		p = p[:maxRW]
		pLen = maxRW
	}
	conn.wop.mode = writeTo
	conn.wop.InitBuf(p)
	conn.wop.sa = addrToSockaddr(conn.family, addr)
	conn.wop.writeHandler = handler
	err := windows.WSASendto(conn.fd, &conn.wop.buf, 1, &conn.wop.qty, conn.wop.flags, conn.wop.sa, &conn.wop.overlapped, nil)
	if err != nil && !errors.Is(windows.ERROR_IO_PENDING, err) {
		err = &net.OpError{
			Op:     writeTo.String(),
			Net:    conn.net,
			Source: conn.localAddr,
			Addr:   conn.remoteAddr,
			Err:    errors.Join(ErrUnexpectedCompletion, wrapSyscallError("WSASendto", err)),
		}
		handler(0, err)
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
	if !conn.ok() {
		handler(0, 0, 0, nil, wrapSyscallError("WSARecvMsg", syscall.EINVAL))
		return
	}
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
	if conn.rop.rsa == nil {
		conn.rop.rsa = new(windows.RawSockaddrAny)
	}
	conn.rop.msg.Name = (*syscall.RawSockaddrAny)(unsafe.Pointer(conn.rop.rsa))
	conn.rop.msg.Namelen = int32(unsafe.Sizeof(*conn.rop.rsa))
	conn.rop.msg.Flags = uint32(0)
	// handle unix
	if conn.family == windows.AF_UNIX {
		conn.rop.flags = readMsgFlags
	}
	conn.rop.rsan = int32(unsafe.Sizeof(*conn.rop.rsa))
	conn.rop.readMsgHandler = handler
	err := windows.WSARecvMsg(conn.fd, &conn.rop.msg, &conn.rop.qty, &conn.rop.overlapped, nil)
	if err != nil && !errors.Is(windows.ERROR_IO_PENDING, err) {
		err = &net.OpError{
			Op:     readMsg.String(),
			Net:    conn.net,
			Source: conn.localAddr,
			Addr:   conn.remoteAddr,
			Err:    errors.Join(ErrUnexpectedCompletion, wrapSyscallError("WSARecvMsg", err)),
		}
		handler(0, 0, 0, nil, err)
		conn.rop.readMsgHandler = nil
	}
	return
}

func (op *operation) completeReadMsg(qty int, err error) {
	sockaddr, sockaddrErr := op.rsa.Sockaddr()
	if sockaddrErr != nil {
		op.readMsgHandler(qty, 0, 0, nil, sockaddrErr)
		op.readMsgHandler = nil
		return
	}
	addr := sockaddrToAddr(op.conn.net, sockaddr)
	if err != nil {
		op.readMsgHandler(qty, 0, 0, addr, &net.OpError{
			Op:     op.mode.String(),
			Net:    op.conn.net,
			Source: op.conn.localAddr,
			Addr:   addr,
			Err:    err,
		})
		op.readMsgHandler = nil
		return
	}
	oobn := int(op.msg.Control.Len)
	flags := int(op.msg.Flags)
	if op.conn.family == windows.AF_UNIX {
		if readMsgFlags == 0 && oobn > 0 {
			oob := op.OOB()
			setReadMsgCloseOnExec(oob[:oobn])
		}
	}
	op.readMsgHandler(qty, oobn, flags, addr, nil)
	op.readMsgHandler = nil
	return
}

func (conn *connection) WriteMsg(p []byte, oob []byte, addr net.Addr, handler WriteMsgHandler) {
	if !conn.ok() {
		handler(0, 0, wrapSyscallError("WSASendMsg", syscall.EINVAL))
		return
	}
	pLen := len(p)
	if pLen == 0 {
		handler(0, 0, ErrEmptyPacket)
		return
	}
	if pLen > maxRW {
		p = p[:maxRW]
	}
	conn.wop.mode = writeMsg
	conn.wop.InitMsg(p, oob)
	if addr != nil {
		if conn.wop.rsa == nil {
			conn.wop.rsa = new(windows.RawSockaddrAny)
		}
		sa := addrToSockaddr(conn.family, addr)
		addrLen, addrErr := sockaddrToRaw(conn.wop.rsa, sa)
		if addrErr != nil {
			handler(0, 0, addrErr)
			return
		}

		conn.wop.msg.Name = (*syscall.RawSockaddrAny)(unsafe.Pointer(conn.wop.rsa))
		conn.wop.msg.Namelen = addrLen
	}
	conn.wop.writeMsgHandler = handler
	err := windows.WSASendMsg(conn.fd, &conn.wop.msg, conn.wop.flags, &conn.wop.qty, &conn.wop.overlapped, nil)
	if err != nil && !errors.Is(windows.ERROR_IO_PENDING, err) {
		err = &net.OpError{
			Op:     writeMsg.String(),
			Net:    conn.net,
			Source: conn.localAddr,
			Addr:   conn.remoteAddr,
			Err:    errors.Join(ErrUnexpectedCompletion, wrapSyscallError("WSASendMsg", err)),
		}
		handler(0, 0, err)
		conn.wop.writeMsgHandler = nil
	}

	return
}

func (op *operation) completeWriteMsg(qty int, err error) {
	sockaddr, sockaddrErr := op.rsa.Sockaddr()
	if sockaddrErr != nil {
		op.writeMsgHandler(qty, 0, sockaddrErr)
		op.writeMsgHandler = nil
		return
	}
	addr := sockaddrToAddr(op.conn.net, sockaddr)
	if err != nil {
		op.writeMsgHandler(qty, 0, &net.OpError{
			Op:     op.mode.String(),
			Net:    op.conn.net,
			Source: op.conn.localAddr,
			Addr:   addr,
			Err:    err,
		})
		op.writeMsgHandler = nil
		return
	}
	oobn := int(op.msg.Control.Len)
	op.writeMsgHandler(qty, oobn, nil)
	op.writeMsgHandler = nil
	return
}
