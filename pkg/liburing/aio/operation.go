//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/liburing"
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
		code:     liburing.IORING_OP_LAST,
		flags:    0,
		resultCh: make(chan Result, resultChanBuffer),
	}
}

const (
	borrowed uint8 = 1 << iota
	discard
	directAlloc
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
	addr2Len uint32
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
	op.sqeFlags |= liburing.IOSQE_IO_LINK
	return op
}

func (op *Operation) WithDirectAlloc(direct bool) *Operation {
	if direct {
		op.flags |= directAlloc
	}
	return op
}

func (op *Operation) reset() {
	op.code = liburing.IORING_OP_LAST
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
	return
}

func (op *Operation) failed(err error) {
	if op.status.CompareAndSwap(ReadyOperationStatus, CompletedOperationStatus) {
		op.resultCh <- Result{0, 0, err}
		return
	}
	if op.status.CompareAndSwap(ProcessingOperationStatus, CompletedOperationStatus) {
		op.resultCh <- Result{0, 0, err}
		return
	}
	if op.status.Load() == HijackedOperationStatus {
		op.resultCh <- Result{0, 0, err}
		return
	}
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

func (op *Operation) completed() bool {
	return op.status.Load() == CompletedOperationStatus && len(op.resultCh) == 0
}

func (op *Operation) Clean() bool {
	if ok := op.status.Load() == CompletedOperationStatus; ok {
		if rLen := len(op.resultCh); rLen > 0 {
			for i := 0; i < rLen; i++ {
				<-op.resultCh
			}
		}
		return true
	}
	return false
}

func (op *Operation) cancelAble() bool {
	return op.status.Load() != CompletedOperationStatus
}

func (op *Operation) prepareAble() bool {
	if ok := op.status.CompareAndSwap(ReadyOperationStatus, ProcessingOperationStatus); ok {
		return true
	}
	if ok := op.status.Load() == HijackedOperationStatus; ok {
		return true
	}
	return false
}

func (op *Operation) releaseAble() bool {
	if hijacked := op.status.Load() == HijackedOperationStatus; hijacked {
		return false
	}
	ok := op.flags&borrowed != 0 && op.flags&discard == 0
	if ok {
		ok = op.Clean()
	}
	return ok
}
