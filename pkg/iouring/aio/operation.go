//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/iouring"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

type Result struct {
	N     int
	Flags uint32
	Err   error
}

const (
	ReadyOperationStatus int64 = iota
	ProcessingOperationStatus
	HijackedOperationStatus
	CompletedOperationStatus
)

func NewOperation(resultChanBuffer int) *Operation {
	if resultChanBuffer < 0 {
		resultChanBuffer = 0
	}
	return &Operation{
		code:     iouring.OpLast,
		flags:    0,
		resultCh: make(chan Result, resultChanBuffer),
	}
}

const (
	borrowed uint8 = 1 << iota
	discard
	directFd
	multishot
)

type Operation struct {
	code     uint8
	cmd      uint8
	flags    uint8
	sqeFlags uint8
	timeout  *syscall.Timespec
	status   atomic.Int64
	resultCh chan Result
	fd       int
	addr     unsafe.Pointer
	addrLen  uint32
	addr2    unsafe.Pointer
	attached *Operation
}

func (op *Operation) Close() {
	op.status.Store(CompletedOperationStatus)
	op.flags |= discard
}

func (op *Operation) Hijack() {
	op.status.Store(HijackedOperationStatus)
}

func (op *Operation) Complete() {
	op.status.Store(CompletedOperationStatus)
}

func (op *Operation) WithDeadline(deadline time.Time) *Operation {
	if deadline.IsZero() {
		return op
	}
	timeout := time.Until(deadline)
	if timeout < 1 {
		timeout = 10 * time.Microsecond
	}
	ns := syscall.NsecToTimespec(timeout.Nanoseconds())
	op.timeout = &ns
	op.sqeFlags |= iouring.SQEIOLink
	return op
}

func (op *Operation) WithDirect(direct bool) *Operation {
	if direct {
		op.flags |= directFd
	}
	return op
}

func (op *Operation) reset() {
	op.code = iouring.OpLast
	op.cmd = 0
	if op.flags&borrowed != 0 {
		op.flags = borrowed
	} else {
		op.flags = 0
	}
	op.sqeFlags = 0
	op.timeout = nil
	op.status.Store(ReadyOperationStatus)

	op.fd = -1
	op.addr = nil
	op.addrLen = 0
	op.addr2 = nil
	op.attached = nil
	return
}

func (op *Operation) failed(err error) {
	op.resultCh <- Result{0, 0, err}
}

func (op *Operation) complete(n int, flags uint32, err error) {
	if ok := op.status.CompareAndSwap(ProcessingOperationStatus, CompletedOperationStatus); ok {
		op.resultCh <- Result{n, flags, err}
		return
	}
	if ok := op.status.Load() == HijackedOperationStatus; ok {
		op.resultCh <- Result{n, flags, err}
		return
	}
	return
}

func (op *Operation) canCancel() bool {
	if ok := op.status.CompareAndSwap(ReadyOperationStatus, CompletedOperationStatus); ok {
		return true
	}
	if ok := op.status.CompareAndSwap(ProcessingOperationStatus, CompletedOperationStatus); ok {
		return true
	}
	if ok := op.status.CompareAndSwap(HijackedOperationStatus, CompletedOperationStatus); ok {
		return true
	}
	return false
}

func (op *Operation) canPrepare() bool {
	if ok := op.status.CompareAndSwap(ReadyOperationStatus, ProcessingOperationStatus); ok {
		return true
	}
	if ok := op.status.Load() == HijackedOperationStatus; ok {
		return true
	}
	return false
}

func (op *Operation) canRelease() bool {
	if hijacked := op.status.Load() == HijackedOperationStatus; hijacked {
		return false
	}
	return op.flags&borrowed != 0 && op.flags&discard == 0
}
