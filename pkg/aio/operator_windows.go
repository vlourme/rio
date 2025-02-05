//go:build windows

package aio

import (
	"golang.org/x/sys/windows"
	"syscall"
)

const (
	maxRW = 1 << 30
)

type SendfileResult struct {
	file    windows.Handle
	curpos  int64
	remain  int64
	written int
}

func newOperator(fd Fd) *Operator {
	return &Operator{
		overlapped: syscall.Overlapped{},
		fd:         fd,
		handle:     -1,
	}
}

type Operator struct {
	overlapped syscall.Overlapped
	fd         Fd
	handle     int
	n          uint32
	oobn       uint32
	rsa        *syscall.RawSockaddrAny
	msg        *windows.WSAMsg
	sfr        SendfileResult
	callback   OperationCallback
	completion OperatorCompletion
}

func (op *Operator) setFd(fd Fd) {
	op.fd = fd
}

func (op *Operator) reset() {
	op.overlapped = syscall.Overlapped{}
	op.fd = nil
	if op.handle == -1 {
		op.handle = -1
	}
	if op.n > 0 {
		op.n = 0
	}
	if op.oobn > 0 {
		op.oobn = 0
	}
	if op.rsa != nil {
		op.rsa = nil
	}
	if op.msg != nil {
		op.msg = nil
	}
	op.sfr.curpos = 0
	op.sfr.remain = 0
	op.sfr.written = 0
	op.sfr.file = windows.InvalidHandle
	if op.callback != nil {
		op.callback = nil
	}
	if op.completion != nil {
		op.completion = nil
	}
}
