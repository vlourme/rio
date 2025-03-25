//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/iouring"
	"github.com/brickingsoft/rio/pkg/iouring/aio/sys"
	"net"
	"os"
	"syscall"
)

func (fd *NetFd) Bind(addr net.Addr) error {
	sa, saErr := sys.AddrToSockaddr(addr)
	if saErr != nil {
		return saErr
	}
	if iouring.VersionEnable(6, 11, 0) && fd.Registered() {
		rsa, rsaLen, rsaErr := sys.SockaddrToRawSockaddrAny(sa)
		if rsaErr != nil {
			return os.NewSyscallError("bind", rsaErr)
		}
		op := fd.vortex.acquireOperation()
		op.PrepareBind(fd, rsa, int(rsaLen))
		_, _, err := fd.vortex.submitAndWait(op)
		fd.vortex.releaseOperation(op)
		return err
	} else {
		if !fd.Installed() {
			if err := fd.Install(); err != nil {
				return err
			}
		}
		if err := syscall.Bind(fd.regular, sa); err != nil {
			return os.NewSyscallError("bind", err)
		}
	}
	return nil
}
