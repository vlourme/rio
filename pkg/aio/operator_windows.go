//go:build windows

package aio

import (
	"golang.org/x/sys/windows"
	"syscall"
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
		n:          0,
		oobn:       0,
		rsa:        nil,
		msg:        nil,
		sfr:        nil,
		callback:   nil,
		completion: nil,
	}
}

type Operator struct {
	overlapped syscall.Overlapped
	processing bool
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
	return op.processing
}

func (op *Operator) begin() {
	op.processing = true
}

func (op *Operator) end() {
	op.processing = false
}

func (op *Operator) reset() {
	op.overlapped = syscall.Overlapped{}
	if op.processing {
		op.processing = false
	}
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
