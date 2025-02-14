package sys

import (
	"github.com/brickingsoft/errors"
	"golang.org/x/sys/unix"
	"os"
	"syscall"
)

func NewSocket(family int, sotype int, protocol int) (sock int, err error) {
	sock, err = syscall.Socket(family, sotype|syscall.SOCK_NONBLOCK|syscall.SOCK_CLOEXEC, protocol)
	if err != nil {
		if errors.Is(err, syscall.EPROTONOSUPPORT) || errors.Is(err, syscall.EINVAL) {
			syscall.ForkLock.RLock()
			sock, err = syscall.Socket(family, sotype, protocol)
			if err == nil {
				syscall.CloseOnExec(sock)
			}
			syscall.ForkLock.RUnlock()
			if err != nil {
				err = os.NewSyscallError("socket", err)
				return
			}
			if err = syscall.SetNonblock(sock, true); err != nil {
				_ = syscall.Close(sock)
				err = os.NewSyscallError("setnonblock", err)
				return
			}
		} else {
			err = os.NewSyscallError("socket", err)
			return
		}
	}
	major, minor := KernelVersion()
	if major >= 4 && minor >= 14 {
		_ = syscall.SetsockoptInt(sock, syscall.SOL_SOCKET, unix.SO_ZEROCOPY, 1)
	}
	return
}
