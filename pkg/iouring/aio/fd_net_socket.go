//go:build linux

package aio

func (fd *NetFd) SetSocketoptInt(level int, optName int, optValue int) (err error) {
	op := fd.vortex.acquireOperation()
	op.PrepareSetSocketoptInt(fd, level, optName, &optValue)
	_, _, err = fd.vortex.submitAndWait(fd.ctx, op)
	fd.vortex.releaseOperation(op)
	return
}

func (fd *NetFd) GetSocketoptInt(level int, optName int) (n int, err error) {
	var optValue int
	op := fd.vortex.acquireOperation()
	op.PrepareGetSocketoptInt(fd, level, optName, &optValue)
	_, _, err = fd.vortex.submitAndWait(fd.ctx, op)
	fd.vortex.releaseOperation(op)
	n = optValue
	return
}
