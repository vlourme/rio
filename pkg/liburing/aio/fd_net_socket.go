//go:build linux

package aio

import "unsafe"

func (fd *NetFd) SetSocketoptInt(level int, optName int, optValue int) (err error) {
	op := fd.vortex.acquireOperation()
	op.PrepareSetSocketoptInt(fd, level, optName, &optValue)
	_, _, err = fd.vortex.submitAndWait(op)
	fd.vortex.releaseOperation(op)
	return
}

func (fd *NetFd) GetSocketoptInt(level int, optName int) (n int, err error) {
	var optValue int
	op := fd.vortex.acquireOperation()
	op.PrepareGetSocketoptInt(fd, level, optName, &optValue)
	_, _, err = fd.vortex.submitAndWait(op)
	fd.vortex.releaseOperation(op)
	n = optValue
	return
}

func (fd *NetFd) SetSocketopt(level int, optName int, optValue unsafe.Pointer, optValueLen int32) (err error) {
	op := fd.vortex.acquireOperation()
	op.PrepareSetSocketopt(fd, level, optName, optValue, optValueLen)
	_, _, err = fd.vortex.submitAndWait(op)
	fd.vortex.releaseOperation(op)
	return
}

func (fd *NetFd) GetSocketopt(level int, optName int, optValue unsafe.Pointer, optValueLen *int32) (err error) {
	op := fd.vortex.acquireOperation()
	op.PrepareGetSocketopt(fd, level, optName, optValue, optValueLen)
	_, _, err = fd.vortex.submitAndWait(op)
	fd.vortex.releaseOperation(op)
	return
}
