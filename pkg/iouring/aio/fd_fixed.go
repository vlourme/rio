//go:build linux

package aio

import "time"

func (fd *NetFd) ReadFixed(buf *FixedBuffer, deadline time.Time) (n int, err error) {
	op := fd.vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareReadFixed(fd, buf)
	n, _, err = fd.vortex.submitAndWait(fd.ctx, op)
	buf.rightShiftWritePosition(n)
	fd.vortex.releaseOperation(op)
	return
}

func (fd *NetFd) WriteFixed(buf *FixedBuffer, deadline time.Time) (n int, err error) {
	op := fd.vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareWriteFixed(fd, buf)
	n, _, err = fd.vortex.submitAndWait(fd.ctx, op)
	buf.rightShiftReadPosition(n)
	fd.vortex.releaseOperation(op)
	return
}

func (fd *NetFd) AcquireBuffer() *FixedBuffer {
	return fd.vortex.AcquireBuffer()
}

func (fd *NetFd) ReleaseBuffer(buf *FixedBuffer) {
	fd.vortex.ReleaseBuffer(buf)
}

func (fd *NetFd) Registered() bool {
	return fd.direct != -1
}

func (fd *NetFd) Register() error {
	if fd.direct > -1 {
		return nil
	}
	direct, regErr := fd.vortex.RegisterFixedFd(fd.regular)
	if regErr != nil {
		return regErr
	}
	fd.direct = direct
	fd.allocated = false
	return nil
}
