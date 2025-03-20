//go:build linux

package aio

import (
	"syscall"
)

func (fd *NetFd) Cancel() {
	if fd.direct != -1 {
		op := fd.vortex.acquireOperation()
		op.PrepareCancelFixedFd(fd.direct)
		_, _, _ = fd.vortex.submitAndWait(fd.ctx, op)
		fd.vortex.releaseOperation(op)
	}
	if fd.regular != -1 {
		op := fd.vortex.acquireOperation()
		op.PrepareCancelFd(fd.regular)
		_, _, _ = fd.vortex.submitAndWait(fd.ctx, op)
		fd.vortex.releaseOperation(op)
	}
	return
}

func (fd *NetFd) Close() error {
	defer fd.cancel()

	fd.Cancel()
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

func (fd *NetFd) closeFd() (err error) {
	op := fd.vortex.acquireOperation()
	op.PrepareClose(fd.regular)
	_, _, err = fd.vortex.submitAndWait(fd.ctx, op)
	fd.vortex.releaseOperation(op)
	return err
}

func (fd *NetFd) closeDirectFd() (err error) {
	op := fd.vortex.acquireOperation()
	op.PrepareCloseDirect(fd.direct)
	_, _, err = fd.vortex.submitAndWait(fd.ctx, op)
	fd.vortex.releaseOperation(op)
	return err
}

func (fd *NetFd) CloseRead() error {
	if fd.direct > -1 {
		op := fd.vortex.acquireOperation()
		op.PrepareCloseRead(fd)
		_, _, err := fd.vortex.submitAndWait(fd.ctx, op)
		fd.vortex.releaseOperation(op)
		return err
	}
	return syscall.Shutdown(fd.regular, syscall.SHUT_RD)
}

func (fd *NetFd) CloseWrite() error {
	if fd.direct > -1 {
		op := fd.vortex.acquireOperation()
		op.PrepareCloseWrite(fd)
		_, _, err := fd.vortex.submitAndWait(fd.ctx, op)
		fd.vortex.releaseOperation(op)
		return err
	}
	return syscall.Shutdown(fd.regular, syscall.SHUT_WR)
}
