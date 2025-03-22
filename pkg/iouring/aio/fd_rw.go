package aio

import (
	"errors"
	"io"
	"net"
	"os"
	"syscall"
)

func (fd *Fd) Read(b []byte) (n int, err error) {
	if fd.IsStream() && len(b) > maxRW {
		b = b[:maxRW]
	}
	if fd.canInAdvance() {
		n, err = syscall.Read(fd.regular, b)
		if err == nil {
			if n == 0 {
				err = io.EOF
			}
			return
		}
		n = 0
		if !errors.Is(err, syscall.EAGAIN) {
			if errors.Is(err, syscall.ECANCELED) {
				err = net.ErrClosed
			} else {
				err = os.NewSyscallError("read", err)
			}
			return
		}
	}

	op := fd.vortex.acquireOperation()
	op.PrepareRead(fd, b)
	n, _, err = fd.vortex.submitAndWait(fd.ctx, op)
	fd.vortex.releaseOperation(op)
	return
}

func (fd *Fd) Write(b []byte) (n int, err error) {
	if fd.IsStream() && len(b) > maxRW {
		b = b[:maxRW]
	}
	nn := 0
	if fd.canInAdvance() {
		n, err = syscall.Write(fd.regular, b)
		if err == nil {
			return
		}
		nn += n
		n = 0
		if !errors.Is(err, syscall.EAGAIN) {
			if errors.Is(err, syscall.ECANCELED) {
				err = net.ErrClosed
			} else {
				err = os.NewSyscallError("write", err)
			}
			return
		}
	}
	if nn > 0 {
		b = b[nn:]
	}

	op := fd.vortex.acquireOperation()
	op.PrepareWrite(fd, b)
	n, _, err = fd.vortex.submitAndWait(fd.ctx, op)
	fd.vortex.releaseOperation(op)
	if err != nil {
		return
	}
	n += nn
	return
}
