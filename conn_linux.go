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

type conn struct {
	ctx          context.Context
	cancel       context.CancelFunc
	fd           *sys.Fd
	vortex       *aio.Vortex
	readTimeout  atomic.Int64
	writeTimeout atomic.Int64
	useZC        bool
}

func (c *conn) Read(b []byte) (n int, err error) {
	if !c.ok() {
		return 0, syscall.EINVAL
	}

	if len(b) == 0 {
		return 0, syscall.EFAULT
	}

	ctx := c.ctx
	fd := c.fd.Socket()
	vortex := c.vortex

	future := vortex.PrepareReceive(ctx, fd, b, time.Duration(c.readTimeout.Load()))

	n, err = future.Await(ctx)
	if err != nil {
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
		return 0, syscall.EFAULT
	}
	ctx := c.ctx
	fd := c.fd.Socket()
	vortex := c.vortex

	if c.useZC {
		future := vortex.PrepareSendZC(ctx, fd, b, time.Duration(c.readTimeout.Load()))
		n, err = future.Await(ctx)
	} else {
		future := vortex.PrepareSend(ctx, fd, b, time.Duration(c.readTimeout.Load()))
		n, err = future.Await(ctx)
	}
	if err != nil {
		err = &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
		return
	}
	return
}

func (c *conn) Close() error {
	if !c.ok() {
		return syscall.EINVAL
	}
	defer func() {
		_ = UnpinVortexes()
	}()
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
	if t.IsZero() {
		c.readTimeout.Store(0)
		c.writeTimeout.Store(0)
		return nil
	}
	timeout := time.Until(t)
	if timeout < 0 {
		timeout = 0
	}
	c.readTimeout.Store(int64(timeout))
	c.writeTimeout.Store(int64(timeout))
	return nil
}

func (c *conn) SetReadDeadline(t time.Time) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	if t.IsZero() {
		c.readTimeout.Store(0)
		return nil
	}
	timeout := time.Until(t)
	if timeout < 0 {
		timeout = 0
	}
	c.readTimeout.Store(int64(timeout))
	return nil
}

func (c *conn) SetWriteDeadline(t time.Time) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	if t.IsZero() {
		c.writeTimeout.Store(0)
		return nil
	}
	timeout := time.Until(t)
	if timeout < 0 {
		timeout = 0
	}
	c.writeTimeout.Store(int64(timeout))
	return nil
}

func (c *conn) SetReadBuffer(bytes int) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	if err := c.fd.SetReadBuffer(bytes); err != nil {
		return &net.OpError{Op: "set", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
	}
	return nil
}

func (c *conn) SetWriteBuffer(bytes int) error {
	if !c.ok() {
		return syscall.EINVAL
	}
	if err := c.fd.SetWriteBuffer(bytes); err != nil {
		return &net.OpError{Op: "set", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
	}
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
