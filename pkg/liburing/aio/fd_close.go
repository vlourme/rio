//go:build linux

package aio

import (
	"syscall"
)

func (fd *Fd) Cancel() {
	if fd.Available() {
		op := AcquireOperation()
		op.PrepareCancelFixedFd(fd.direct)
		_, _, _ = poller.SubmitAndWait(op)
		ReleaseOperation(op)
		if fd.regular != -1 {
			op = AcquireOperation()
			op.PrepareCancelFd(fd.regular)
			_, _, _ = poller.SubmitAndWait(op)
			ReleaseOperation(op)
		}
	}
	return
}

func (fd *Fd) Close() error {
	if fd.Available() {
		fd.Cancel()
		err := fd.closeDirectFd()
		if fd.regular != -1 {
			_ = fd.closeRegularFd()
		}
		Unpin()
		return err
	}
	return ErrFdUnavailable
}

func (fd *Fd) closeDirectFd() (err error) {
	op := AcquireOperation()
	op.PrepareCloseDirect(fd.direct)
	_, _, err = poller.SubmitAndWait(op)
	ReleaseOperation(op)
	if err == nil {
		fd.direct = -1
	}
	return
}

func (fd *Fd) closeRegularFd() (err error) {
	op := AcquireOperation()
	op.PrepareClose(fd.regular)
	_, _, err = poller.SubmitAndWait(op)
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
