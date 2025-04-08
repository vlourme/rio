//go:build linux

package aio

import (
	"errors"
	"fmt"
	"github.com/brickingsoft/rio/pkg/liburing/aio/sys"
	"syscall"
	"time"
)

const maxRW = 1 << 30

type Fd struct {
	regular       int
	direct        int
	isStream      bool
	zeroReadIsEOF bool
	readDeadline  time.Time
	writeDeadline time.Time
	multishot     bool
	eventLoop     *EventLoop
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

func (fd *Fd) SetReadDeadline(t time.Time) {
	fd.readDeadline = t
}

func (fd *Fd) SetWriteDeadline(t time.Time) {
	fd.writeDeadline = t
}

func (fd *Fd) Registered() bool {
	return fd.direct != -1
}

func (fd *Fd) Installed() bool {
	return fd.regular != -1
}

func (fd *Fd) Install() (err error) {
	if fd.regular != -1 {
		return nil
	}
	if fd.direct == -1 {
		err = errors.New("fd is not directed")
		return
	}
	op := fd.eventLoop.resource.AcquireOperation()
	op.PrepareFixedFdInstall(fd.direct)
	fd.regular, _, err = fd.eventLoop.SubmitAndWait(op)
	fd.eventLoop.resource.ReleaseOperation(op)
	return
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
