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

const maxRW = 1 << 30

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
	conn = &connection{net: network, fd: fd}
	conn.rop.conn = conn
	conn.wop.conn = conn
	conn.zeroReadIsEOF = sotype != syscall.SOCK_DGRAM && sotype != syscall.SOCK_RAW
	return
}

type connection struct {
	cphandle      windows.Handle
	fd            windows.Handle
	localAddr     net.Addr
	remoteAddr    net.Addr
	net           string
	zeroReadIsEOF bool
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

const (
	defaultTCPTimeout = 1000 * time.Millisecond
)

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
