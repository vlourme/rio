//go:build linux

package rio

import (
	"context"
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/iouring/aio"
	"io"
	"net"
	"syscall"
)

// AcquireRegisteredBuffer implements the FixedReaderWriter AcquireRegisteredBuffer method.
func (c *conn) AcquireRegisteredBuffer() *aio.FixedBuffer {
	if !c.ok() {
		return nil
	}
	return c.vortex.AcquireBuffer()
}

// ReleaseRegisteredBuffer implements the FixedReaderWriter ReleaseRegisteredBuffer method.
func (c *conn) ReleaseRegisteredBuffer(buf *aio.FixedBuffer) {
	if !c.ok() {
		return
	}
	c.vortex.ReleaseBuffer(buf)
}

// ReadFixed implements the FixedReaderWriter ReadFixed method.
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

	var (
		ctx      = c.ctx
		fd       int
		sqeFlags uint8
	)
	if c.directFd > -1 {
		fd = c.directFd
		sqeFlags = iouring.SQEFixedFile
	} else {
		fd = c.fd.Socket()
	}
	vortex := c.vortex
	deadline := c.deadline(ctx, c.readDeadline)

	n, err = vortex.ReadFixed(ctx, fd, buf, deadline, sqeFlags)
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

// WriteFixed implements the FixedReaderWriter WriteFixed method.
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

	var (
		ctx      = c.ctx
		fd       int
		sqeFlags uint8
	)
	if c.directFd > -1 {
		fd = c.directFd
		sqeFlags = iouring.SQEFixedFile
	} else {
		fd = c.fd.Socket()
	}
	vortex := c.vortex
	deadline := c.deadline(ctx, c.writeDeadline)

	n, err = vortex.WriteFixed(ctx, fd, buf, deadline, sqeFlags)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			err = net.ErrClosed
		}
		err = &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: err}
		return
	}
	return
}

// RegisterDirectFd implements the FixedConn RegisterDirectFd method.
func (c *conn) RegisterDirectFd() error {
	if !c.ok() {
		return syscall.EINVAL
	}
	if c.directFd > -1 {
		return nil
	}
	fd := c.fd.Socket()
	vortex := c.vortex
	direct, regErr := vortex.RegisterFixedFd(fd)
	if regErr != nil {
		return &net.OpError{Op: "write", Net: c.fd.Net(), Source: c.fd.LocalAddr(), Addr: c.fd.RemoteAddr(), Err: regErr}
	}
	c.directFd = direct
	return nil
}

// DirectFd implements the FixedConn DirectFd method.
func (c *conn) DirectFd() int {
	return c.directFd
}
