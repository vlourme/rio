//go:build linux

package aio

import (
	"io"
)

func (fd *Fd) Read(b []byte) (n int, err error) {
	if fd.IsStream() && len(b) > maxRW {
		b = b[:maxRW]
	}
	op := fd.eventLoop.resource.AcquireOperation()
	op.WithDeadline(fd.eventLoop.resource, fd.readDeadline).PrepareRead(fd, b)
	n, _, err = fd.eventLoop.SubmitAndWait(op)
	fd.eventLoop.resource.ReleaseOperation(op)
	if n == 0 && err == nil && fd.ZeroReadIsEOF() {
		err = io.EOF
	}
	return
}

func (fd *Fd) Write(b []byte) (n int, err error) {
	if fd.IsStream() && len(b) > maxRW {
		b = b[:maxRW]
	}
	op := fd.eventLoop.resource.AcquireOperation()
	op.WithDeadline(fd.eventLoop.resource, fd.writeDeadline).PrepareWrite(fd, b)
	n, _, err = fd.eventLoop.SubmitAndWait(op)
	fd.eventLoop.resource.ReleaseOperation(op)
	if err != nil {
		return
	}
	return
}
