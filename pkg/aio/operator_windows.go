//go:build windows

package aio

import (
	"golang.org/x/sys/windows"
	"sync/atomic"
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
		processing: atomic.Bool{},
		fd:         fd,
		handle:     -1,
	}
}

type Operator struct {
	overlapped syscall.Overlapped
	processing atomic.Bool
	fd         Fd
	handle     int
	n          uint32
	oobn       uint32
	rsa        *syscall.RawSockaddrAny
	msg        *windows.WSAMsg
	sfr        *SendfileResult
	callback   OperationCallback
	completion OperatorCompletion
}

func (op *Operator) Processing() bool {
	return op.processing.Load()
}

func (op *Operator) begin() {
	op.processing.Store(true)
}

func (op *Operator) end() {
	op.processing.Store(false)
}

func (op *Operator) reset() {
	op.overlapped = syscall.Overlapped{}
	op.processing.Store(false)
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
	if op.sfr != nil {
		op.sfr = nil
	}
	if op.callback != nil {
		op.callback = nil
	}
	if op.completion != nil {
		op.completion = nil
	}
}
