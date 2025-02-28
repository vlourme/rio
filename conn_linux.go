//go:build linux

package rio

import (
	"context"
	"errors"
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

type conn struct {
	ctx           context.Context
	cancel        context.CancelFunc
	fd            *sys.Fd
	vortex        *aio.Vortex
	readDeadline  time.Time
	writeDeadline time.Time
	readBuffer    atomic.Int64
	writeBuffer   atomic.Int64
	useZC         bool
	accepted      bool
}

func (c *conn) Context() context.Context {
	return c.ctx
}

func (c *conn) Read(b []byte) (n int, err error) {
	if !c.ok() {
		return 0, syscall.EINVAL
	}

	if len(b) == 0 {
		return 0, &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}

	ctx := c.ctx
	fd := c.fd.Socket()
	vortex := c.vortex
	deadline := c.readDeadline

RETRY:
	future := vortex.PrepareReceive(ctx, fd, b, deadline)
	n, err = future.Await(ctx)
	if err != nil {
		if errors.Is(err, syscall.EBUSY) {
			if !deadline.IsZero() && deadline.Before(time.Now()) {
				err = &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: aio.Timeout}
				return
			}
			goto RETRY
		}
		err = &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
		return
	}
	if n == 0 && c.fd.ZeroReadIsEOF() {
		err = io.EOF
		return
	}
	return
}

func (c *conn) Write(b []byte) (n int, err error) {
	if !c.ok() {
		return 0, syscall.EINVAL
	}
	if len(b) == 0 {
		return 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}
	ctx := c.ctx
	fd := c.fd.Socket()
	vortex := c.vortex
	deadline := c.writeDeadline

RETRY:
	if c.useZC {
		future := vortex.PrepareSendZC(ctx, fd, b, deadline)
		n, err = future.Await(ctx)
	} else {
		future := vortex.PrepareSend(ctx, fd, b, deadline)
		n, err = future.Await(ctx)
	}

	if err != nil {
		if errors.Is(err, syscall.EBUSY) {
			if !deadline.IsZero() && deadline.Before(time.Now()) {
				err = &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: aio.Timeout}
				return
			}
			goto RETRY
		}
		err = &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
		return
	}
	return
}

func (c *conn) Close() error {
	if !c.ok() {
		return syscall.EINVAL
	}
	defer func(c *conn) {
		if !c.accepted {
			_ = UnpinVortexes()
		}
	}(c)
	defer c.cancel()

	if err := c.fd.Close(); err != nil {
		return &net.OpError{Op: "close", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (c *conn) LocalAddr() net.Addr {
	if !c.ok() {
		return nil
	}
	return c.fd.LocalAddr()
}

func (c *conn) RemoteAddr() net.Addr {
	if !c.ok() {
		return nil
	}
	return c.fd.RemoteAddr()
}

func (c *conn) SetDeadline(t time.Time) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	if err := c.SetReadDeadline(t); err != nil {
		return err
	}
	if err := c.SetWriteDeadline(t); err != nil {
		return err
	}
	return nil
}

func (c *conn) SetReadDeadline(t time.Time) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	if t.IsZero() {
		c.readDeadline = t
		return nil
	}
	if t.Before(time.Now()) {
		return &net.OpError{Op: "set", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: errors.New("set deadline too early")}
	}
	c.readDeadline = t
	return nil
}

func (c *conn) SetWriteDeadline(t time.Time) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	if t.IsZero() {
		c.writeDeadline = t
		return nil
	}
	if t.Before(time.Now()) {
		return &net.OpError{Op: "set", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: errors.New("set deadline too early")}
	}
	c.writeDeadline = t
	return nil
}

func (c *conn) ReadBuffer() (int, error) {
	if !c.ok() {
		return 0, syscall.EINVAL
	}
	if n := c.readBuffer.Load(); n != 0 {
		return int(n), nil
	}
	n, err := c.fd.ReadBuffer()
	if err != nil {
		return 0, &net.OpError{Op: "get", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
	}
	c.readBuffer.Store(int64(n))
	return n, nil
}

func (c *conn) SetReadBuffer(bytes int) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	if err := c.fd.SetReadBuffer(bytes); err != nil {
		return &net.OpError{Op: "set", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
	}
	c.readBuffer.Store(int64(bytes))
	return nil
}

func (c *conn) WriteBuffer() (int, error) {
	if !c.ok() {
		return 0, syscall.EINVAL
	}
	if n := c.writeBuffer.Load(); n != 0 {
		return int(n), nil
	}
	n, err := c.fd.WriteBuffer()
	if err != nil {
		return 0, &net.OpError{Op: "get", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
	}
	c.writeBuffer.Store(int64(n))
	return n, nil
}

func (c *conn) SetWriteBuffer(bytes int) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	if err := c.fd.SetWriteBuffer(bytes); err != nil {
		return &net.OpError{Op: "set", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
	}
	c.writeBuffer.Store(int64(bytes))
	return nil
}

func (c *conn) File() (f *os.File, err error) {
	if !c.ok() {
		return nil, syscall.EINVAL
	}
	f, err = c.file()
	if err != nil {
		err = &net.OpError{Op: "file", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
	}
	return
}

func (c *conn) file() (*os.File, error) {
	ns, call, err := c.fd.Dup()
	if err != nil {
		if call != "" {
			err = os.NewSyscallError(call, err)
		}
		return nil, err
	}
	f := os.NewFile(uintptr(ns), c.fd.Name())
	return f, nil
}

func (c *conn) SyscallConn() (syscall.RawConn, error) {
	if !c.ok() {
		return nil, syscall.EINVAL
	}
	return newRawConn(c.fd), nil
}

func (c *conn) ok() bool { return c != nil && c.fd != nil }

func newRawConn(fd *sys.Fd) syscall.RawConn {
	return &rawConn{fd: fd}
}

type ctrlCtxFn func(ctx context.Context, network string, address string, raw syscall.RawConn) error

type rawConn struct {
	fd *sys.Fd
}

func (c *rawConn) Control(f func(fd uintptr)) error {
	fd := c.fd.Socket()
	f(uintptr(fd))
	runtime.KeepAlive(c.fd)
	return nil
}

func (c *rawConn) Read(f func(fd uintptr) (done bool)) (err error) {
	fd := c.fd.Socket()
	for {
		if f(uintptr(fd)) {
			break
		}
	}
	runtime.KeepAlive(c.fd)
	return
}

func (c *rawConn) Write(f func(fd uintptr) (done bool)) (err error) {
	fd := c.fd.Socket()
	for {
		if f(uintptr(fd)) {
			break
		}
	}
	runtime.KeepAlive(c.fd)
	return
}
