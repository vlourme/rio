//go:build linux

package rio

import (
	"context"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"github.com/brickingsoft/rio/pkg/sys"
	"io"
	"net"
	"os"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
)

type connection struct {
	ctx          context.Context
	cancel       context.CancelFunc
	fd           *sys.Fd
	vortex       *aio.Vortex
	readTimeout  atomic.Int64
	writeTimeout atomic.Int64
	useZC        bool
}

func (conn *connection) Read(b []byte) (n int, err error) {
	if len(b) == 0 {
		return
	}

	ctx := conn.ctx
	fd := conn.fd.Socket()
	vortex := conn.vortex

	future := vortex.PrepareReceive(ctx, fd, b, time.Duration(conn.readTimeout.Load()))

	n, err = future.Await(ctx)
	if err != nil {
		err = &net.OpError{Op: "read", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
		return
	}
	if n == 0 && conn.fd.ZeroReadIsEOF() {
		err = io.EOF
		return
	}
	return
}

func (conn *connection) Write(b []byte) (n int, err error) {
	if len(b) == 0 {
		return
	}
	ctx := conn.ctx
	fd := conn.fd.Socket()
	vortex := conn.vortex

	if conn.useZC {
		future := vortex.PrepareSendZC(ctx, fd, b, time.Duration(conn.readTimeout.Load()))
		n, err = future.Await(ctx)
	} else {
		future := vortex.PrepareSend(ctx, fd, b, time.Duration(conn.readTimeout.Load()))
		n, err = future.Await(ctx)
	}
	if err != nil {
		err = &net.OpError{Op: "write", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
		return
	}
	return
}

func (conn *connection) Close() error {
	defer func() {
		_ = UnpinVortexes()
	}()
	defer conn.cancel()

	if err := conn.fd.Close(); err != nil {
		return &net.OpError{Op: "close", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (conn *connection) LocalAddr() net.Addr {
	return conn.fd.LocalAddr()
}

func (conn *connection) RemoteAddr() net.Addr {
	return conn.fd.RemoteAddr()
}

func (conn *connection) SetDeadline(t time.Time) error {
	if t.IsZero() {
		conn.readTimeout.Store(0)
		conn.writeTimeout.Store(0)
		return nil
	}
	timeout := time.Until(t)
	if timeout < 0 {
		timeout = 0
	}
	conn.readTimeout.Store(int64(timeout))
	conn.writeTimeout.Store(int64(timeout))
	return nil
}

func (conn *connection) SetReadDeadline(t time.Time) error {
	if t.IsZero() {
		conn.readTimeout.Store(0)
		return nil
	}
	timeout := time.Until(t)
	if timeout < 0 {
		timeout = 0
	}
	conn.readTimeout.Store(int64(timeout))
	return nil
}

func (conn *connection) SetWriteDeadline(t time.Time) error {
	if t.IsZero() {
		conn.writeTimeout.Store(0)
		return nil
	}
	timeout := time.Until(t)
	if timeout < 0 {
		timeout = 0
	}
	conn.writeTimeout.Store(int64(timeout))
	return nil
}

func (conn *connection) SetReadBuffer(bytes int) error {
	if err := conn.fd.SetReadBuffer(bytes); err != nil {
		return &net.OpError{Op: "set", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (conn *connection) SetWriteBuffer(bytes int) error {
	if err := conn.fd.SetWriteBuffer(bytes); err != nil {
		return &net.OpError{Op: "set", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (conn *connection) File() (f *os.File, err error) {
	f, err = conn.file()
	if err != nil {
		err = &net.OpError{Op: "file", Net: conn.fd.Net(), Source: conn.fd.LocalAddr(), Addr: conn.fd.RemoteAddr(), Err: err}
	}
	return
}

func (conn *connection) file() (*os.File, error) {
	ns, call, err := conn.fd.Dup()
	if err != nil {
		if call != "" {
			err = os.NewSyscallError(call, err)
		}
		return nil, err
	}
	f := os.NewFile(uintptr(ns), conn.fd.Name())
	return f, nil
}

func newRawConnection(fd *sys.Fd) syscall.RawConn {
	return &rawConnection{fd: fd}
}

type ctrlCtxFn func(ctx context.Context, network string, address string, raw syscall.RawConn) error

type rawConnection struct {
	fd *sys.Fd
}

func (conn *rawConnection) Control(f func(fd uintptr)) error {
	fd := conn.fd.Socket()
	f(uintptr(fd))
	runtime.KeepAlive(conn.fd)
	return nil
}

func (conn *rawConnection) Read(f func(fd uintptr) (done bool)) (err error) {
	fd := conn.fd.Socket()
	for {
		if f(uintptr(fd)) {
			break
		}
	}
	runtime.KeepAlive(conn.fd)
	return
}

func (conn *rawConnection) Write(f func(fd uintptr) (done bool)) (err error) {
	fd := conn.fd.Socket()
	for {
		if f(uintptr(fd)) {
			break
		}
	}
	runtime.KeepAlive(conn.fd)
	return
}
