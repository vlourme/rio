//go:build linux

package aio

import (
	"errors"
	"fmt"
	"github.com/brickingsoft/rio/pkg/liburing"
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"sync"
	"syscall"
)

const maxRW = 1 << 30

type Fd struct {
	regular       int
	direct        int
	isStream      bool
	zeroReadIsEOF bool
	locker        sync.Mutex
	vortex        *Vortex
}

func (fd *Fd) FileDescriptor() (n int, direct bool) {
	if fd.Registered() {
		n = fd.direct
		direct = true
		return
	}
	return fd.regular, false
}

func (fd *Fd) RegularFd() int {
	return fd.regular
}

func (fd *Fd) DirectFd() int {
	return fd.direct
}

func (fd *Fd) IsStream() bool {
	return fd.isStream
}

func (fd *Fd) ZeroReadIsEOF() bool {
	return fd.zeroReadIsEOF
}

func (fd *Fd) Name() string {
	return fmt.Sprintf("[fd:%d][direct:%d]", fd.regular, fd.direct)
}

func (fd *Fd) OperationSupported(op uint8) bool {
	return fd.vortex.OpSupported(op)
}

func (fd *Fd) SendZCSupported() bool {
	return fd.vortex.OpSupported(liburing.IORING_OP_SEND_ZC)
}

func (fd *Fd) SendMsgZCSupported() bool {
	return fd.vortex.OpSupported(liburing.IORING_OP_SENDMSG_ZC)
}

func (fd *Fd) Registered() bool {
	return fd.direct != -1
}

func (fd *Fd) Installed() bool {
	return fd.regular != -1
}

func (fd *Fd) Install() (err error) {
	fd.locker.Lock()
	defer fd.locker.Unlock()
	if fd.regular != -1 {
		return nil
	}
	if fd.direct == -1 {
		err = errors.New("fd is not directed")
		return
	}
	regular, installErr := fd.vortex.FixedFdInstall(fd.direct)
	if installErr != nil {
		err = installErr
		return
	}
	fd.regular = regular
	return
}

func (fd *Fd) Vortex() *Vortex {
	return fd.vortex
}

func (fd *Fd) SyscallConn() (syscall.RawConn, error) {
	if !fd.Installed() {
		if err := fd.Install(); err != nil {
			return nil, err
		}
	}
	return sys.NewRawConn(fd.RegularFd()), nil
}

func (fd *Fd) Dup() (int, string, error) {
	if !fd.Installed() {
		if err := fd.Install(); err != nil {
			return 0, "", err
		}
	}
	return sys.DupCloseOnExec(fd.regular)
}
