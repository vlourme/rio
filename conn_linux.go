//go:build linux

package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
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
	fdFixed       bool
	fileIndex     int
	sqeFlags      uint8
	vortex        *aio.Vortex
	readDeadline  time.Time
	writeDeadline time.Time
	readBuffer    atomic.Int64
	writeBuffer   atomic.Int64
	pinned        bool
	useSendZC     bool
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
	fd := 0
	if c.fdFixed {
		fd = c.fileIndex
	} else {
		fd = c.fd.Socket()
	}
	vortex := c.vortex
	deadline := c.deadline(ctx, c.readDeadline)

	n, err = vortex.Receive(ctx, fd, b, deadline, c.sqeFlags)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			err = net.ErrClosed
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
	fd := 0
	if c.fdFixed {
		fd = c.fileIndex
	} else {
		fd = c.fd.Socket()
	}
	vortex := c.vortex
	deadline := c.deadline(ctx, c.writeDeadline)

	if c.useSendZC {
		n, err = vortex.SendZC(ctx, fd, b, deadline, c.sqeFlags)
	} else {
		n, err = vortex.Send(ctx, fd, b, deadline, c.sqeFlags)
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			err = net.ErrClosed
		}
		err = &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
		return
	}
	return
}

func (c *conn) InstallFixedFd() (err error) {
	if !c.ok() {
		return syscall.EINVAL
	}

	if c.fdFixed {
		return nil
	}
	ctx := c.ctx
	vortex := c.vortex

	sock := c.fd.Socket()

	file, regErr := vortex.RegisterFixedFd(ctx, sock)
	if regErr != nil {
		return &net.OpError{Op: "install_fixed_file", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: regErr}
	}

	c.fdFixed = true
	c.fileIndex = file
	c.sqeFlags |= iouring.SQEFixedFile
	return nil
}

func (c *conn) FixedFdInstalled() bool {
	return c.fdFixed
}

func (c *conn) Close() error {
	if !c.ok() {
		return syscall.EINVAL
	}
	defer c.cancel()

	ctx := c.ctx
	vortex := c.vortex

	var err error
	if c.fdFixed {
		err = vortex.CloseDirect(ctx, c.fileIndex)
		_ = vortex.UnregisterFixedFd(c.fileIndex)
	} else {
		fd := c.fd.Socket()
		err = vortex.Close(ctx, fd)
	}

	if err != nil {
		fd := c.fd.Socket()
		err = syscall.Close(fd)
		if c.pinned {
			if unpinErr := aio.Release(vortex); unpinErr != nil {
				if err == nil {
					err = unpinErr
				}
			}
		}
		if err != nil {
			return &net.OpError{Op: "close", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
		}
		return nil
	}

	if c.pinned {
		if unpinErr := aio.Release(vortex); unpinErr != nil {
			return &net.OpError{Op: "close", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: unpinErr}
		}
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

func (c *conn) deadline(ctx context.Context, deadline time.Time) time.Time {
	if ctxDeadline, ok := ctx.Deadline(); ok {
		if deadline.IsZero() {
			deadline = ctxDeadline
		} else if ctxDeadline.Before(deadline) {
			deadline = ctxDeadline
		}
	}
	return deadline
}

func (c *conn) SyscallConn() (syscall.RawConn, error) {
	if !c.ok() {
		return nil, syscall.EINVAL
	}
	return newRawConn(c.fd), nil
}

func (c *conn) ok() bool { return c != nil && c.fd != nil }

func (c *conn) AcquireRegisteredBuffer() *aio.FixedBuffer {
	if !c.ok() {
		return nil
	}
	return c.vortex.AcquireBuffer()
}

func (c *conn) ReleaseRegisteredBuffer(buf *aio.FixedBuffer) {
	if !c.ok() {
		return
	}
	c.vortex.ReleaseBuffer(buf)
}

func (c *conn) ReadFixed(buf *aio.FixedBuffer) (n int, err error) {
	if !c.ok() {
		return 0, syscall.EINVAL
	}
	if buf == nil {
		return 0, &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}
	if !buf.Validate() {
		return 0, &net.OpError{Op: "read", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}

	ctx := c.ctx
	fd := 0
	if c.fdFixed {
		fd = c.fileIndex
	} else {
		fd = c.fd.Socket()
	}
	vortex := c.vortex
	deadline := c.deadline(ctx, c.readDeadline)

	n, err = vortex.ReadFixed(ctx, fd, buf, deadline, c.sqeFlags)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			err = net.ErrClosed
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

func (c *conn) WriteFixed(buf *aio.FixedBuffer) (n int, err error) {
	if !c.ok() {
		return 0, syscall.EINVAL
	}
	if buf == nil {
		return 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}
	if !buf.Validate() {
		return 0, &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: syscall.EINVAL}
	}

	ctx := c.ctx
	fd := 0
	if c.fdFixed {
		fd = c.fileIndex
	} else {
		fd = c.fd.Socket()
	}
	vortex := c.vortex
	deadline := c.deadline(ctx, c.writeDeadline)

	n, err = vortex.WriteFixed(ctx, fd, buf, deadline, c.sqeFlags)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			err = net.ErrClosed
		}
		err = &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
		return
	}
	return
}

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
