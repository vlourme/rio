//go:build windows

package sockets

import (
	"errors"
	"golang.org/x/sys/windows"
	"net"
	"os"
	"syscall"
	"time"
)

const (
	maxRW             = 1 << 30
	defaultTCPTimeout = 1000 * time.Millisecond
)

func wrapSyscallError(name string, err error) error {
	var errno windows.Errno
	if errors.As(err, &errno) {
		err = os.NewSyscallError(name, err)
	}
	return err
}

func netSocket(family int, sotype int, protocol int, ipv6only bool) (fd windows.Handle, err error) {
	// socket
	fd, err = windows.WSASocket(int32(family), int32(sotype), int32(protocol), nil, 0, windows.WSA_FLAG_OVERLAPPED|windows.WSA_FLAG_NO_HANDLE_INHERIT)
	if err != nil {
		err = wrapSyscallError("WSASocket", err)
		return
	}
	// set default opts
	setDefaultSockOptsErr := setDefaultSockopts(fd, family, sotype, ipv6only)
	if setDefaultSockOptsErr != nil {
		err = setDefaultSockOptsErr
		_ = windows.Closesocket(fd)
		return
	}
	return
}

func newConnection(network string, family int, sotype int, protocol int, ipv6only bool) (conn *connection, err error) {
	// socket
	fd, fdErr := netSocket(family, sotype, protocol, ipv6only)
	if fdErr != nil {
		err = fdErr
		return
	}
	// conn
	conn = &connection{net: network, fd: fd, family: family, sotype: sotype, ipv6only: ipv6only}
	conn.rop.conn = conn
	conn.wop.conn = conn
	conn.zeroReadIsEOF = sotype != syscall.SOCK_DGRAM && sotype != syscall.SOCK_RAW
	return
}

type connection struct {
	cphandle      windows.Handle
	fd            windows.Handle
	family        int
	sotype        int
	ipv6only      bool
	zeroReadIsEOF bool
	net           string
	localAddr     net.Addr
	remoteAddr    net.Addr
	rop           operation
	wop           operation
}

func (conn *connection) LocalAddr() (addr net.Addr) {
	addr = conn.localAddr
	return
}

func (conn *connection) RemoteAddr() (addr net.Addr) {
	addr = conn.remoteAddr
	return
}

func (conn *connection) SetDeadline(deadline time.Time) (err error) {
	timeout := deadline.Sub(time.Now())
	if timeout == 0 {
		timeout = defaultTCPTimeout
	} else if timeout < 0 {
		return nil
	}
	millis := int(roundDurationUp(timeout, time.Millisecond))
	err = windows.SetsockoptInt(conn.fd, windows.SOL_SOCKET, windows.SO_RCVTIMEO, millis)
	if err != nil {
		err = wrapSyscallError("setsockopt", err)
		return
	}
	// SO_SNDTIMEO was not supported
	return
}

func (conn *connection) SetReadDeadline(deadline time.Time) (err error) {
	timeout := deadline.Sub(time.Now())
	if timeout == 0 {
		timeout = defaultTCPTimeout
	} else if timeout < 0 {
		return nil
	}
	millis := int(roundDurationUp(timeout, time.Millisecond))
	err = windows.SetsockoptInt(conn.fd, windows.SOL_SOCKET, windows.SO_RCVTIMEO, millis)
	if err != nil {
		err = wrapSyscallError("setsockopt", err)
		return
	}
	return
}

func (conn *connection) SetWriteDeadline(_ time.Time) (err error) {
	// SO_SNDTIMEO was not supported
	return
}

func (conn *connection) Close() (err error) {
	_ = windows.Shutdown(conn.fd, 2)
	err = windows.Closesocket(conn.fd)
	if err != nil {
		err = &net.OpError{
			Op:     "close",
			Net:    conn.net,
			Source: conn.localAddr,
			Addr:   conn.remoteAddr,
			Err:    err,
		}
	}
	conn.rop.conn = nil
	conn.wop.conn = nil
	return
}

func connect(network string, family int, addr net.Addr, ipv6only bool, proto int, handler DialHandler) {
	// conn
	conn, connErr := newConnection(network, family, windows.SOCK_STREAM, proto, ipv6only)
	if connErr != nil {
		handler(nil, connErr)
		return
	}
	conn.rop.mode = dial
	conn.rop.dialHandler = handler
	// lsa
	var lsa windows.Sockaddr
	if ipv6only {
		lsa = &windows.SockaddrInet6{}
	} else {
		lsa = &windows.SockaddrInet4{}
	}
	// bind
	bindErr := windows.Bind(conn.fd, lsa)
	if bindErr != nil {
		_ = windows.Closesocket(conn.fd)
		handler(nil, os.NewSyscallError("bind", bindErr))
		return
	}
	// CreateIoCompletionPort
	cphandle, createErr := windows.CreateIoCompletionPort(conn.fd, iocp, key, 0)
	if createErr != nil {
		_ = windows.Closesocket(conn.fd)
		handler(nil, wrapSyscallError("createIoCompletionPort", createErr))
		conn.rop.dialHandler = nil
		return
	}
	conn.cphandle = cphandle
	// ra
	rsa := addrToSockaddr(family, addr)
	// overlapped
	overlapped := &conn.rop.overlapped
	// dial
	connectErr := windows.ConnectEx(conn.fd, rsa, nil, 0, nil, overlapped)
	if connectErr != nil && !errors.Is(connectErr, windows.ERROR_IO_PENDING) {
		_ = windows.Closesocket(conn.fd)
		handler(nil, wrapSyscallError("ConnectEx", connectErr))
		conn.rop.dialHandler = nil
		return
	}
}

func (op *operation) completeDial(_ int, err error) {
	if err != nil {
		op.dialHandler(nil, os.NewSyscallError("ConnectEx", err))
		op.dialHandler = nil
		return
	}
	conn := op.conn
	// set SO_UPDATE_CONNECT_CONTEXT
	setSocketOptErr := windows.Setsockopt(
		conn.fd,
		windows.SOL_SOCKET, windows.SO_UPDATE_CONNECT_CONTEXT,
		nil,
		0,
	)
	if setSocketOptErr != nil {
		op.dialHandler(nil, os.NewSyscallError("setsockopt", setSocketOptErr))
		op.dialHandler = nil
		return
	}
	// get addr
	lsa, lsaErr := windows.Getsockname(conn.fd)
	if lsaErr != nil {
		op.dialHandler(nil, os.NewSyscallError("getsockname", lsaErr))
		op.dialHandler = nil
		return
	}
	la := sockaddrToAddr(conn.net, lsa)
	conn.localAddr = la
	rsa, rsaErr := windows.Getpeername(op.conn.fd)
	if rsaErr != nil {
		op.dialHandler(nil, os.NewSyscallError("getsockname", rsaErr))
		op.dialHandler = nil
		return
	}
	ra := sockaddrToAddr(conn.net, rsa)
	conn.remoteAddr = ra
	// callback
	op.dialHandler(conn, nil)
	op.dialHandler = nil
}
