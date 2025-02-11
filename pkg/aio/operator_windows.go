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

type Operator struct {
	overlapped syscall.Overlapped
	received   atomic.Bool
	fd         Fd
	handle     int
	n          uint32
	msg        windows.WSAMsg
	sfr        SendfileResult
	callback   OperationCallback
	completion OperatorCompletion
}

func (op *Operator) setFd(fd Fd) {
	op.fd = fd
}

func (op *Operator) reset() {
	// overlapped
	op.overlapped.OffsetHigh = 0
	op.overlapped.Offset = 0
	op.overlapped.HEvent = 0
	op.overlapped.Internal = 0
	op.overlapped.InternalHigh = 0
	// received
	op.received.Store(false)
	// fd
	op.fd = nil
	op.handle = 0
	// qty
	op.n = 0
	// msg
	op.msg.Name = nil
	op.msg.Namelen = 0
	op.msg.Buffers = nil
	op.msg.BufferCount = 0
	op.msg.Flags = 0
	op.msg.Control.Buf = nil
	op.msg.Control.Len = 0
	// sfr
	op.sfr.curpos = 0
	op.sfr.remain = 0
	op.sfr.written = 0
	op.sfr.file = windows.InvalidHandle
	// fn
	op.callback = nil
	op.completion = nil
	return
}
