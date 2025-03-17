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
	status     atomic.Int64
	kind       uint8
	borrowed   bool
	resultCh   chan Result
	deadline   time.Time
	multishot  bool
	directMode bool
	filedIndex int
	sqeFlags   uint8
	fd         int
	msg        syscall.Msghdr
	pipe       pipeRequest
	ptr        unsafe.Pointer
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

func (op *Operation) UseMultishot() {
	op.multishot = true
}

func (op *Operation) WithDeadline(deadline time.Time) *Operation {
	op.deadline = deadline
	return op
}

func (op *Operation) Timeout() time.Duration {
	if deadline := op.deadline; !deadline.IsZero() {
		return time.Until(deadline)
	}
	return 0
}

func (op *Operation) WithSQEFlags(flags uint8) *Operation {
	op.sqeFlags |= flags
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

func (op *Operation) PrepareNop() (err error) {
	op.kind = iouring.OpNop
	return
}

func (op *Operation) PrepareConnect(fd int, addr *syscall.RawSockaddrAny, addrLen int) {
	op.kind = iouring.OpConnect
	op.fd = fd
	op.msg.Name = (*byte)(unsafe.Pointer(addr))
	op.msg.Namelen = uint32(addrLen)
	return
}

func (op *Operation) PrepareAcceptMultishot(fd int, addr *syscall.RawSockaddrAny, addrLen int) {
	op.kind = iouring.OpAccept
	op.multishot = true
	op.fd = fd
	op.msg.Name = (*byte)(unsafe.Pointer(addr))
	op.msg.Namelen = uint32(addrLen)
	return
}

func (op *Operation) PrepareAccept(fd int, addr *syscall.RawSockaddrAny, addrLen int) {
	op.kind = iouring.OpAccept
	op.fd = fd
	op.msg.Name = (*byte)(unsafe.Pointer(addr))
	op.msg.Namelen = uint32(addrLen)
	return
}

func (op *Operation) PrepareClose(fd int) {
	op.kind = iouring.OpClose
	op.fd = fd
}

func (op *Operation) PrepareReadFixed(fd int, buf *FixedBuffer) {
	op.kind = iouring.OpReadFixed
	op.fd = fd
	op.msg.Name = &buf.value[buf.rPos]
	op.msg.Namelen = uint32(len(buf.value) - buf.rPos)
	op.msg.Iovlen = uint64(buf.index)
	return
}

func (op *Operation) PrepareWriteFixed(fd int, buf *FixedBuffer) {
	op.kind = iouring.OpWriteFixed
	op.fd = fd
	op.msg.Name = &buf.value[buf.rPos]
	op.msg.Namelen = uint32(buf.Length())
	op.msg.Iovlen = uint64(buf.index)
	return
}

func (op *Operation) PrepareReceive(fd int, b []byte) {
	op.kind = iouring.OpRecv
	op.fd = fd
	op.msg.Name = &b[0]
	op.msg.Namelen = uint32(len(b))
	return
}

func (op *Operation) PrepareSend(fd int, b []byte) {
	op.kind = iouring.OpSend
	op.fd = fd
	op.msg.Name = &b[0]
	op.msg.Namelen = uint32(len(b))
	return
}

func (op *Operation) PrepareSendZC(fd int, b []byte) {
	op.kind = iouring.OpSendZC
	op.fd = fd
	op.msg.Name = &b[0]
	op.msg.Namelen = uint32(len(b))
	return
}

func (op *Operation) PrepareReceiveMsg(fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32) {
	op.kind = iouring.OpRecvmsg
	op.fd = fd
	op.setMsg(b, oob, addr, addrLen, flags)
	return
}

func (op *Operation) PrepareSendMsg(fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32) {
	op.kind = iouring.OpSendmsg
	op.fd = fd
	op.setMsg(b, oob, addr, addrLen, flags)
	return
}

func (op *Operation) PrepareSendMsgZC(fd int, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32) {
	op.kind = iouring.OpSendMsgZC
	op.fd = fd
	op.setMsg(b, oob, addr, addrLen, flags)
	return
}

func (op *Operation) PrepareSplice(fdIn int, offIn int64, fdOut int, offOut int64, nbytes uint32, flags uint32) {
	op.kind = iouring.OpSplice
	op.pipe.fdIn = fdIn
	op.pipe.offIn = offIn
	op.pipe.fdOut = fdOut
	op.pipe.offOut = offOut
	op.pipe.nbytes = nbytes
	op.pipe.spliceFlags = flags
}

func (op *Operation) PrepareTee(fdIn int, fdOut int, nbytes uint32, flags uint32) {
	op.kind = iouring.OpTee
	op.pipe.fdIn = fdIn
	op.pipe.fdOut = fdOut
	op.pipe.nbytes = nbytes
	op.pipe.spliceFlags = flags
}

func (op *Operation) PrepareCancel(target *Operation) {
	op.kind = iouring.OpAsyncCancel
	op.ptr = unsafe.Pointer(target)
}

func (op *Operation) PrepareCancelFd(fd int) {
	op.kind = iouring.OpAsyncCancel
	op.fd = fd
}

func (op *Operation) PrepareCancelFixedFd(fileIndex int) {
	op.kind = iouring.OpAsyncCancel
	op.filedIndex = fileIndex
}

func (op *Operation) PrepareFixedFdInstall(fd int) {
	op.kind = iouring.OPFixedFdInstall
	op.fd = fd
}

func (op *Operation) reset() {
	// kind
	op.kind = iouring.OpLast
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
	// deadline
	op.deadline = time.Time{}
	// ptr
	if op.ptr != nil {
		op.ptr = nil
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

func (op *Operation) Addr() (addr *syscall.RawSockaddrAny, addrLen int) {
	addr = (*syscall.RawSockaddrAny)(unsafe.Pointer(op.msg.Name))
	addrLen = int(op.msg.Namelen)
	return
}

func (op *Operation) Control() []byte {
	if op.msg.Control == nil || op.msg.Controllen == 0 {
		return nil
	}
	return unsafe.Slice(op.msg.Control, op.msg.Controllen)
}

func (op *Operation) ControlLen() int {
	return int(op.msg.Controllen)
}

func (op *Operation) Flags() int {
	return int(op.msg.Flags)
}

func (op *Operation) setResult(n int, flags uint32, err error) {
	op.resultCh <- Result{n, flags, err}
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

func (op *Operation) canSetResult() bool {
	if ok := op.status.CompareAndSwap(ProcessingOperationStatus, CompletedOperationStatus); ok {
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
