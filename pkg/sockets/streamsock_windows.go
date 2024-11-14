//go:build windows

package sockets

import (
	"context"
	"errors"
	"golang.org/x/sys/windows"
	"net"
	"os"
	"time"
	"unsafe"
)

func newListener(network string, family int, addr net.Addr, ipv6only bool, proto int) (ln *listener, err error) {
	// create listener fd
	fd, fdErr := netSocket(family, windows.SOCK_STREAM, proto, ipv6only)
	if fdErr != nil {
		err = os.NewSyscallError("WSASocket", fdErr)
		return
	}
	// bind
	sockaddr := addrToSockaddr(family, addr)
	bindErr := windows.Bind(fd, sockaddr)
	if bindErr != nil {
		err = os.NewSyscallError("Bind", bindErr)
		_ = windows.Closesocket(fd)
		return
	}
	// listen
	listenErr := windows.Listen(fd, windows.SOMAXCONN)
	if listenErr != nil {
		err = os.NewSyscallError("Listen", listenErr)
		_ = windows.Closesocket(fd)
		return
	}
	// lsa
	lsa, getLSAErr := windows.Getsockname(fd)
	if getLSAErr != nil {
		err = os.NewSyscallError("Getsockname", getLSAErr)
		_ = windows.Closesocket(fd)
		return
	}
	addr = sockaddrToAddr(network, lsa)

	// create listener iocp
	cphandler, createListenIOCPErr := createSubIoCompletionPort(fd)
	if createListenIOCPErr != nil {
		err = os.NewSyscallError("CreateIoCompletionPort", createListenIOCPErr)
		_ = windows.Closesocket(fd)
		return
	}

	// create listener
	ln = &listener{
		cphandle: cphandler,
		fd:       fd,
		family:   family,
		ipv6only: ipv6only,
		net:      network,
		addr:     addr,
	}
	return
}

type listener struct {
	cphandle windows.Handle
	fd       windows.Handle
	family   int
	ipv6only bool
	net      string
	addr     net.Addr
}

func (ln *listener) Addr() (addr net.Addr) {
	addr = ln.addr
	return
}

func (ln *listener) Accept(handler AcceptHandler) {
	// conn
	conn, connErr := newConnection(ln.net, windows.AF_INET, windows.SOCK_STREAM, 0, ln.ipv6only)
	if connErr != nil {
		handler(nil, connErr)
		return
	}
	// op
	conn.rop.mode = accept
	conn.rop.handle = ln.fd
	conn.rop.iocp = ln.cphandle
	conn.rop.acceptHandler = handler
	// sa
	var rawsa [2]windows.RawSockaddrAny
	lsan := uint32(unsafe.Sizeof(rawsa[1]))
	rsa := &rawsa[0]
	rsan := uint32(unsafe.Sizeof(rawsa[0]))
	// overlapped
	overlapped := &conn.rop.overlapped
	// accept
	acceptErr := windows.AcceptEx(
		ln.fd, conn.fd,
		(*byte)(unsafe.Pointer(rsa)), 0,
		lsan+16, rsan+16,
		&conn.rop.qty, overlapped,
	)
	if acceptErr != nil && !errors.Is(windows.ERROR_IO_PENDING, acceptErr) {
		_ = windows.Closesocket(conn.fd)
		handler(nil, wrapSyscallError("AcceptEx", acceptErr))
		conn.rop.acceptHandler = nil
	}
}

func (ln *listener) Close() (err error) {
	// close socket
	closeSockErr := windows.Closesocket(ln.fd)
	if closeSockErr != nil {
		err = wrapSyscallError("closesocket", closeSockErr)
	}
	return
}

func (op *operation) completeAccept(_ int, err error) {
	if err != nil {
		op.acceptHandler(nil, os.NewSyscallError("AcceptEx", err))
		op.acceptHandler = nil
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
		return
	}
	// get addr
	lsa, lsaErr := windows.Getsockname(conn.fd)
	if lsaErr != nil {
		op.acceptHandler(nil, os.NewSyscallError("getsockname", lsaErr))
		op.acceptHandler = nil
		return
	}
	la := sockaddrToAddr(conn.net, lsa)
	conn.localAddr = la
	rsa, rsaErr := windows.Getpeername(op.conn.fd)
	if rsaErr != nil {
		op.acceptHandler(nil, os.NewSyscallError("getsockname", rsaErr))
		op.acceptHandler = nil
		return
	}
	ra := sockaddrToAddr(conn.net, rsa)
	conn.remoteAddr = ra
	// CreateIoCompletionPort
	cphandle, createErr := windows.CreateIoCompletionPort(op.conn.fd, op.iocp, key, 0)
	if createErr != nil {
		op.acceptHandler(nil, os.NewSyscallError("createIoCompletionPort", createErr))
		op.acceptHandler = nil
		return
	}
	conn.cphandle = cphandle
	// callback
	op.acceptHandler(conn, nil)
	op.acceptHandler = nil
	return
}

func (conn *connection) SetNoDelay(noDelay bool) (err error) {
	err = windows.SetsockoptInt(conn.fd, windows.IPPROTO_TCP, windows.TCP_NODELAY, boolint(noDelay))
	if err != nil {
		err = wrapSyscallError("setsockopt", err)
		return
	}
	return
}

func (conn *connection) SetLinger(sec int) (err error) {
	var l windows.Linger
	if sec >= 0 {
		l.Onoff = 1
		l.Linger = int32(sec)
	} else {
		l.Onoff = 0
		l.Linger = 0
	}
	err = windows.SetsockoptLinger(conn.fd, windows.SOL_SOCKET, windows.SO_LINGER, &l)
	if err != nil {
		err = wrapSyscallError("setsockopt", err)
		return
	}
	return
}

func (conn *connection) SetKeepAlive(keepalive bool) (err error) {
	err = windows.SetsockoptInt(conn.fd, windows.SOL_SOCKET, windows.SO_KEEPALIVE, boolint(keepalive))
	if err != nil {
		err = wrapSyscallError("setsockopt", err)
		return
	}
	return
}

// *********************************************************************************************************************

const (
	defaultTCPKeepAliveIdle = 5 * time.Second
)

func (conn *connection) SetKeepAlivePeriod(period time.Duration) (err error) {
	if period == 0 {
		period = defaultTCPKeepAliveIdle
	} else if period < 0 {
		return nil
	}
	secs := int(roundDurationUp(period, time.Second))
	err = windows.SetsockoptInt(conn.fd, windows.IPPROTO_TCP, windows.TCP_KEEPIDLE, secs)
	if err != nil {
		err = wrapSyscallError("setsockopt", err)
		return
	}
	return
}

func (conn *connection) Read(p []byte, handler ReadHandler) {
	pLen := len(p)
	if pLen == 0 {
		handler(0, ErrEmptyPacket)
		return
	} else if pLen > maxRW {
		p = p[:maxRW]
		pLen = maxRW
	}
	conn.rop.mode = read
	conn.rop.buf.Buf = &p[0]
	conn.rop.buf.Len = uint32(pLen)
	conn.rop.readHandler = handler
	err := windows.WSARecv(conn.fd, &conn.rop.buf, 1, &conn.rop.qty, &conn.rop.flags, &conn.rop.overlapped, nil)
	if err != nil && !errors.Is(windows.ERROR_IO_PENDING, err) {
		if errors.Is(windows.ERROR_TIMEOUT, err) {
			err = context.DeadlineExceeded
		}
		handler(0, wrapSyscallError("WSARecv", err))
		conn.rop.readHandler = nil
	}
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

func (conn *connection) Write(p []byte, handler WriteHandler) {
	pLen := len(p)
	if pLen == 0 {
		handler(0, ErrEmptyPacket)
		return
	} else if pLen > maxRW {
		p = p[:maxRW]
		pLen = maxRW
	}
	conn.wop.mode = write
	conn.wop.buf.Buf = &p[0]
	conn.wop.buf.Len = uint32(pLen)
	conn.wop.writeHandler = handler
	err := windows.WSASend(conn.fd, &conn.wop.buf, 1, &conn.wop.qty, conn.wop.flags, &conn.wop.overlapped, nil)
	if err != nil && !errors.Is(windows.ERROR_IO_PENDING, err) {
		if errors.Is(windows.ERROR_TIMEOUT, err) {
			err = context.DeadlineExceeded
		}
		handler(0, wrapSyscallError("WSASend", err))
		conn.wop.writeHandler = nil
	}
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
