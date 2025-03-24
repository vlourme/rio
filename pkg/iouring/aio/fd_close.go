//go:build linux

package aio

import (
	"syscall"
)

func (fd *Fd) Cancel() {
	if fd.direct != -1 {
		op := fd.vortex.acquireOperation()
		op.PrepareCancelFixedFd(fd.direct)
		_, _, _ = fd.vortex.submitAndWait(op)
		fd.vortex.releaseOperation(op)
	}
	if fd.regular != -1 {
		op := fd.vortex.acquireOperation()
		op.PrepareCancelFd(fd.regular)
		_, _, _ = fd.vortex.submitAndWait(op)
		fd.vortex.releaseOperation(op)
	}
	return
}

func (fd *Fd) Close() error {
	if fd.direct != -1 {
		err := fd.closeDirectFd()
		if !fd.allocated {
			_ = fd.vortex.UnregisterFixedFd(fd.direct)
		}
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
	op := fd.vortex.acquireOperation()
	op.PrepareClose(fd.regular)
	_, _, err = fd.vortex.submitAndWait(op)
	fd.vortex.releaseOperation(op)
	return err
}

func (fd *Fd) closeDirectFd() (err error) {
	op := fd.vortex.acquireOperation()
	op.PrepareCloseDirect(fd.direct)
	_, _, err = fd.vortex.submitAndWait(op)
	fd.vortex.releaseOperation(op)
	return err
}
