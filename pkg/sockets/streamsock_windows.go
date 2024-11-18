//go:build windows

package sockets

import (
	"errors"
	"golang.org/x/sys/windows"
	"net"
	"os"
	"runtime"
	"sync"
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
	runtime.SetFinalizer(ln, (*listener).Close)
	return
}

type listener struct {
	cphandle   windows.Handle
	fd         windows.Handle
	family     int
	ipv6only   bool
	net        string
	addr       net.Addr
	unlink     bool
	unlinkOnce sync.Once
}

func (ln *listener) Addr() (addr net.Addr) {
	if ln.family == windows.AF_UNIX {
		addr = ln.addr
	}
	return
}

func (ln *listener) SetUnlinkOnClose(unlink bool) {
	ln.unlink = unlink
}

func (ln *listener) Close() (err error) {
	runtime.SetFinalizer(ln, nil)
	// check unix
	if ln.family == windows.AF_UNIX && ln.unlink {
		ln.unlinkOnce.Do(func() {
			unixAddr, isUnix := ln.addr.(*net.UnixAddr)
			if isUnix {
				if path := unixAddr.String(); path[0] != '@' {
					_ = windows.Unlink(path)
				}
			}
		})
	}
	// close socket
	closeSockErr := windows.Closesocket(ln.fd)
	runtime.KeepAlive(ln)
	if closeSockErr != nil {
		err = wrapSyscallError("closesocket", closeSockErr)
	}
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
		_ = conn.Close()
		handler(nil, wrapSyscallError("AcceptEx", acceptErr))
		conn.rop.acceptHandler = nil
	}
}

func (op *operation) completeAccept(_ int, err error) {
	if err != nil {
		op.acceptHandler(nil, os.NewSyscallError("AcceptEx", err))
		op.acceptHandler = nil
		_ = op.conn.Close()
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
		_ = op.conn.Close()
		return
	}
	// get addr
	lsa, lsaErr := windows.Getsockname(conn.fd)
	if lsaErr != nil {
		op.acceptHandler(nil, os.NewSyscallError("getsockname", lsaErr))
		op.acceptHandler = nil
		_ = op.conn.Close()
		return
	}
	la := sockaddrToAddr(conn.net, lsa)
	conn.localAddr = la
	rsa, rsaErr := windows.Getpeername(op.conn.fd)
	if rsaErr != nil {
		op.acceptHandler(nil, os.NewSyscallError("getsockname", rsaErr))
		op.acceptHandler = nil
		_ = op.conn.Close()
		return
	}
	ra := sockaddrToAddr(conn.net, rsa)
	conn.remoteAddr = ra
	// CreateIoCompletionPort
	cphandle, createErr := windows.CreateIoCompletionPort(op.conn.fd, op.iocp, key, 0)
	if createErr != nil {
		op.acceptHandler(nil, os.NewSyscallError("createIoCompletionPort", createErr))
		op.acceptHandler = nil
		_ = op.conn.Close()
		return
	}
	conn.cphandle = cphandle
	// connected
	conn.connected.Store(true)
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
