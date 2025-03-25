//go:build linux

package aio

import (
	"io"
	"time"
)

func (fd *Fd) ReadFixed(buf *FixedBuffer, deadline time.Time) (n int, err error) {
	op := fd.vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareReadFixed(fd, buf)
	n, _, err = fd.vortex.submitAndWait(op)
	buf.rightShiftWritePosition(n)
	fd.vortex.releaseOperation(op)
	if n == 0 && err == nil && fd.ZeroReadIsEOF() {
		err = io.EOF
	}
	return
}

func (fd *Fd) WriteFixed(buf *FixedBuffer, deadline time.Time) (n int, err error) {
	op := fd.vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareWriteFixed(fd, buf)
	n, _, err = fd.vortex.submitAndWait(op)
	buf.rightShiftReadPosition(n)
	fd.vortex.releaseOperation(op)
	return
}

func (fd *Fd) AcquireBuffer() *FixedBuffer {
	return fd.vortex.AcquireBuffer()
}

func (fd *Fd) ReleaseBuffer(buf *FixedBuffer) {
	fd.vortex.ReleaseBuffer(buf)
}
