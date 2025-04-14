//go:build linux

package aio

import (
	"io"
)

func (fd *Fd) Read(b []byte) (n int, err error) {
	if fd.IsStream() && len(b) > maxRW {
		b = b[:maxRW]
	}
	op := AcquireOperationWithDeadline(fd.readDeadline)
	n, _, err = fd.eventLoop.SubmitAndWait(op)
	ReleaseOperation(op)
	if n == 0 && err == nil && fd.ZeroReadIsEOF() {
		err = io.EOF
	}
	return
}

func (fd *Fd) Write(b []byte) (n int, err error) {
	if fd.IsStream() && len(b) > maxRW {
		b = b[:maxRW]
	}
	op := AcquireOperationWithDeadline(fd.writeDeadline)
	n, _, err = fd.eventLoop.SubmitAndWait(op)
	ReleaseOperation(op)
	if err != nil {
		return
	}
	return
}
