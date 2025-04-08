//go:build linux

package aio

import (
	"syscall"
)

func (fd *Fd) Cancel() {
	if fd.direct != -1 {
		op := fd.eventLoop.resource.AcquireOperation()
		op.PrepareCancelFixedFd(fd.direct)
		_, _, _ = fd.eventLoop.SubmitAndWait(op)
		fd.eventLoop.resource.ReleaseOperation(op)
	}
	if fd.regular != -1 {
		op := fd.eventLoop.resource.AcquireOperation()
		op.PrepareCancelFd(fd.regular)
		_, _, _ = fd.eventLoop.SubmitAndWait(op)
		fd.eventLoop.resource.ReleaseOperation(op)
	}
	return
}

func (fd *Fd) Close() error {
	if fd.direct != -1 {
		err := fd.closeDirectFd()
		if fd.regular != -1 {
			_ = syscall.Close(fd.regular)
		}
		return err
	}
	if fd.regular != -1 {
		if err := fd.closeFd(); err != nil {
			_ = syscall.Close(fd.regular)
			return err
		}
	}
	return nil
}

func (fd *Fd) closeFd() (err error) {
	op := fd.eventLoop.resource.AcquireOperation()
	op.PrepareClose(fd.regular)
	_, _, err = fd.eventLoop.SubmitAndWait(op)
	fd.eventLoop.resource.ReleaseOperation(op)
	return
}

func (fd *Fd) closeDirectFd() (err error) {
	op := fd.eventLoop.resource.AcquireOperation()
	op.PrepareCloseDirect(fd.direct)
	_, _, err = fd.eventLoop.SubmitAndWait(op)
	fd.eventLoop.resource.ReleaseOperation(op)
	return
}
