//go:build linux

package aio

import (
	"syscall"
	"time"
)

func (fd *NetFd) Connect(addr *syscall.RawSockaddrAny, addrLen int, deadline time.Time) (n int, err error) {
	op := fd.vortex.acquireOperation()
	op.WithDeadline(deadline).PrepareConnect(fd, addr, addrLen)
	n, _, err = fd.vortex.submitAndWait(fd.ctx, op)
	fd.vortex.releaseOperation(op)
	return
}
