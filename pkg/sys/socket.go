package sys

import (
	"github.com/brickingsoft/errors"
	"os"
	"syscall"
)

func NewSocket(family int, sotype int, protocol int) (fd *Fd, err error) {
	sock, sockErr := syscall.Socket(family, sotype|syscall.SOCK_NONBLOCK|syscall.SOCK_CLOEXEC, protocol)
	if sockErr != nil {
		if errors.Is(err, syscall.EPROTONOSUPPORT) || errors.Is(err, syscall.EINVAL) {
			syscall.ForkLock.RLock()
			sock, sockErr = syscall.Socket(family, sotype, protocol)
			if sockErr == nil {
				syscall.CloseOnExec(sock)
			}
			syscall.ForkLock.RUnlock()
			if sockErr != nil {
				err = os.NewSyscallError("socket", err)
				return
			}
			if sockErr = syscall.SetNonblock(sock, true); sockErr != nil {
				_ = syscall.Close(sock)
				err = os.NewSyscallError("setnonblock", err)
				return
			}
		} else {
			err = os.NewSyscallError("socket", err)
			return
		}
	}
	fd = &Fd{
		sock:   sock,
		family: family,
		sotype: sotype,
		net:    "",
		laddr:  nil,
		raddr:  nil,
	}
	return
}
