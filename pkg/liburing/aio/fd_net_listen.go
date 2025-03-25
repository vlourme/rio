//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"os"
	"syscall"
)

func (fd *NetFd) Listen() (err error) {
	backlog := sys.MaxListenerBacklog()
	if liburing.VersionEnable(6, 11, 0) && fd.Registered() {
		op := fd.vortex.acquireOperation()
		op.PrepareListen(fd, backlog)
		_, _, err = fd.vortex.submitAndWait(op)
		fd.vortex.releaseOperation(op)
		return err
	} else {
		if !fd.Installed() {
			if err = fd.Install(); err != nil {
				return err
			}
		}
		if err = syscall.Listen(fd.regular, backlog); err != nil {
			err = os.NewSyscallError("listen", err)
			return
		}
	}
	return
}
