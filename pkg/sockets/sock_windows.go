//go:build windows

package sockets

import (
	"errors"
	"golang.org/x/sys/windows"
	"net"
	"os"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	maxRW = 1 << 30
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
	// try set SO_BROADCAST
	if sotype == syscall.SOCK_DGRAM {
		if family == windows.AF_INET || family == windows.AF_INET6 {
			setBroadcaseErr := windows.SetsockoptInt(fd, windows.SOL_SOCKET, windows.SO_BROADCAST, 1)
			if setBroadcaseErr != nil {
				err = setBroadcaseErr
				_ = windows.Closesocket(fd)
				return
			}
		}
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
	conn = &connection{net: network, fd: fd, family: family, sotype: sotype, ipv6only: ipv6only, connected: atomic.Bool{}}
	runtime.SetFinalizer(conn, (*connection).Closesocket)
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
	connected     atomic.Bool
	rto           time.Duration
	wto           time.Duration
	rop           operation
	wop           operation
}

func (conn *connection) ok() bool {
	return conn != nil && conn.connected.Load()
}

func (conn *connection) LocalAddr() (addr net.Addr) {
	addr = conn.localAddr
	return
}

func (conn *connection) RemoteAddr() (addr net.Addr) {
	addr = conn.remoteAddr
	return
}

func (conn *connection) SetReadBuffer(n int) (err error) {
	err = windows.SetsockoptInt(conn.fd, windows.SOL_SOCKET, windows.SO_RCVBUF, n)
	if err != nil {
		err = wrapSyscallError("setsockopt", err)
	}
	return
}

func (conn *connection) SetWriteBuffer(n int) (err error) {
	err = windows.SetsockoptInt(conn.fd, windows.SOL_SOCKET, windows.SO_SNDBUF, n)
	if err != nil {
		err = wrapSyscallError("setsockopt", err)
	}
	return
}

func (conn *connection) SetReadTimeout(d time.Duration) (err error) {
	conn.rto = d
	return
}

func (conn *connection) SetWriteTimeout(d time.Duration) (err error) {
	conn.wto = d
	return
}

func (conn *connection) Close(handler CloseHandler) {
	err := conn.Closesocket()
	if handler != nil {
		handler(err)
	}
	return
}

func (conn *connection) Closesocket() (err error) {
	runtime.SetFinalizer(conn, nil)
	_ = windows.Shutdown(conn.fd, 2)
	err = windows.Closesocket(conn.fd)
	runtime.KeepAlive(conn)
	conn.fd = 0
	if err != nil {
		err = &net.OpError{
			Op:     "close",
			Net:    conn.net,
			Source: conn.localAddr,
			Addr:   conn.remoteAddr,
			Err:    err,
		}
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
	if !conn.ok() {
		handler(0, wrapSyscallError("WSARecv", syscall.EINVAL))
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
	conn.rop.mode = read
	conn.rop.InitBuf(p)
	conn.rop.readHandler = handler

	if timeout := conn.rto; timeout > 0 {
		timer := getOperationTimer()
		conn.rop.timer = timer
		timer.Start(timeout, &conn.rop)
	}

	err := windows.WSARecv(conn.fd, &conn.rop.buf, 1, &conn.rop.qty, &conn.rop.flags, &conn.rop.overlapped, nil)
	if err != nil && !errors.Is(windows.ERROR_IO_PENDING, err) {
		// handle err
		err = &net.OpError{
			Op:     read.String(),
			Net:    conn.net,
			Source: conn.localAddr,
			Addr:   conn.remoteAddr,
			Err:    errors.Join(ErrUnexpectedCompletion, wrapSyscallError("WSARecv", err)),
		}
		handler(0, err)
		// clean
		conn.rop.readHandler = nil
		timer := conn.rop.timer
		timer.Done()
		putOperationTimer(timer)
		conn.rop.timer = nil
	}
}

func (op *operation) completeRead(qty int, err error) {
	if err != nil {
		op.readHandler(qty, &net.OpError{
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
	conn.wop.mode = write
	conn.wop.InitBuf(p)
	conn.wop.writeHandler = handler

	if timeout := conn.wto; timeout > 0 {
		timer := getOperationTimer()
		conn.wop.timer = timer
		timer.Start(timeout, &conn.wop)
	}

	err := windows.WSASend(conn.fd, &conn.wop.buf, 1, &conn.wop.qty, conn.wop.flags, &conn.wop.overlapped, nil)
	if err != nil && !errors.Is(windows.ERROR_IO_PENDING, err) {
		// handle err
		err = &net.OpError{
			Op:     write.String(),
			Net:    conn.net,
			Source: conn.localAddr,
			Addr:   conn.remoteAddr,
			Err:    errors.Join(ErrUnexpectedCompletion, wrapSyscallError("WSASend", err)),
		}
		handler(0, err)
		// clean
		conn.wop.writeHandler = nil
		timer := conn.wop.timer
		timer.Done()
		putOperationTimer(timer)
		conn.wop.timer = nil
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

// *********************************************************************************************************************

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
		_ = conn.Closesocket()
		handler(nil, os.NewSyscallError("bind", bindErr))
		return
	}
	// CreateIoCompletionPort
	cphandle, createErr := createSubIoCompletionPort(conn.fd)
	if createErr != nil {
		_ = conn.Closesocket()
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
		_ = conn.Closesocket()
		err := &net.OpError{
			Op:     dial.String(),
			Net:    network,
			Source: sockaddrToAddr("network", lsa),
			Addr:   addr,
			Err:    errors.Join(ErrUnexpectedCompletion, wrapSyscallError("ConnectEx", connectErr)),
		}
		handler(nil, err)
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
	// connected
	conn.connected.Store(true)
	// callback
	op.dialHandler(conn, nil)
	op.dialHandler = nil
}
