//go:build linux

package aio

import (
	"syscall"
)

func (fd *Fd) Cancel() {
	fd.cancelDirect()
	if fd.regular != -1 {
		fd.cancelRegular()
	}
	return
}

func (fd *Fd) cancelDirect() {
	op := AcquireOperation()
	op.PrepareCancelFixedFd(fd.direct)
	_, _, _ = fd.eventLoop.SubmitAndWait(op)
	ReleaseOperation(op)
}

func (fd *Fd) cancelRegular() {
	op := AcquireOperation()
	op.PrepareCancelFd(fd.regular)
	_, _, _ = fd.eventLoop.SubmitAndWait(op)
	ReleaseOperation(op)
}

func (fd *Fd) Close() error {
	fd.Cancel()
	err := fd.closeDirectFd()
	if fd.regular != -1 {
		_ = fd.closeRegularFd()
	}
	return err
}

func (fd *Fd) closeDirectFd() (err error) {
	op := AcquireOperation()
	op.PrepareCloseDirect(fd.direct)
	_, _, err = fd.eventLoop.SubmitAndWait(op)
	ReleaseOperation(op)
	if err == nil {
		fd.direct = -1
	}
	return
}

func (fd *Fd) closeRegularFd() (err error) {
	op := AcquireOperation()
	op.PrepareClose(fd.regular)
	_, _, err = fd.eventLoop.SubmitAndWait(op)
	ReleaseOperation(op)
	if err != nil {
		if err = syscall.Close(fd.regular); err == nil {
			fd.regular = -1
		}
	} else {
		fd.regular = -1
	}
	return
}
