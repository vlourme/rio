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

type pipeRequest struct {
	fdIn        int
	offIn       int64
	fdOut       int
	offOut      int64
	nbytes      uint32
	spliceFlags uint32
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
		kind:     iouring.OpLast,
		resultCh: make(chan Result, resultChanBuffer),
	}
}

type Operation struct {
	kind        uint8
	subKind     int
	borrowed    bool
	status      atomic.Int64
	resultCh    chan Result
	timeout     *syscall.Timespec
	multishot   bool
	directMode  bool
	filedIndex  int
	sqeFlags    uint8
	fd          int
	ptr         unsafe.Pointer
	pipe        pipeRequest
	msg         syscall.Msghdr
	linkTimeout *Operation
}

func (op *Operation) Close() {
	op.status.Store(CompletedOperationStatus)
	op.borrowed = false
}

func (op *Operation) Hijack() {
	op.status.Store(HijackedOperationStatus)
}

func (op *Operation) Complete() {
	op.status.Store(CompletedOperationStatus)
}

func (op *Operation) Prepared() {
	op.status.Store(ProcessingOperationStatus)
}

func (op *Operation) UseMultishot() {
	op.multishot = true
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
	op.directMode = direct
	return op
}

func (op *Operation) WithFiledIndex(index uint32) *Operation {
	op.filedIndex = int(index)
	return op
}

func (op *Operation) reset() {
	// kind
	op.kind = iouring.OpLast
	op.subKind = -1
	// status
	op.status.Store(ReadyOperationStatus)
	// multishot
	op.multishot = false
	// direct
	op.directMode = false
	// file index
	op.filedIndex = -1
	// sqe flags
	op.sqeFlags = 0
	// fd
	op.fd = -1
	// msg
	op.msg.Name = nil
	op.msg.Namelen = 0
	if op.msg.Iov != nil {
		op.msg.Iov = nil
		op.msg.Iovlen = 0
		op.msg.Control = nil
		op.msg.Controllen = 0
		op.msg.Flags = 0
	}
	// pipe
	if op.pipe.fdIn != 0 {
		op.pipe.fdIn = 0
		op.pipe.offIn = 0
		op.pipe.fdOut = 0
		op.pipe.offOut = 0
		op.pipe.nbytes = 0
		op.pipe.spliceFlags = 0
	}
	// timeout
	op.timeout = nil
	// ptr
	if op.ptr != nil {
		op.ptr = nil
	}
	// linkTimeout
	if op.linkTimeout != nil {
		op.linkTimeout = nil
	}
	return
}

func (op *Operation) setMsg(b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32) {
	bLen := len(b)
	oobLen := len(oob)
	if bLen > 0 {
		op.msg.Iov = &syscall.Iovec{
			Base: &b[0],
			Len:  uint64(bLen),
		}
		op.msg.Iovlen = 1
	}
	if oobLen > 0 {
		op.msg.Control = &oob[0]
		op.msg.SetControllen(oobLen)
	}
	if addr != nil {
		op.msg.Name = (*byte)(unsafe.Pointer(addr))
		op.msg.Namelen = uint32(addrLen)
	}
	op.msg.Flags = flags
	return
}

func (op *Operation) setResult(n int, flags uint32, err error) {
	op.resultCh <- Result{n, flags, err}
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
	return op.borrowed
}
