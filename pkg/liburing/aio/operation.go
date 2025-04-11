//go:build linux

package aio

import (
	"errors"
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

type OperationHandler interface {
	Handle(n int, flags uint32, err error)
}

const (
	ReadyOperationStatus int64 = iota
	ProcessingOperationStatus
	HijackedOperationStatus
	CompletedOperationStatus
)

const (
	op_f_borrowed uint8 = 1 << iota // todo remove
	op_f_timeout                    // todo remove
	op_f_discard                    // todo remove
	op_f_direct_alloc
	op_f_multishot
	op_f_with_handler // todo remove
	op_f_noexec
)

const (
	op_cmd_close uint8 = iota + 1
	op_cmd_msg_ring
	op_cmd_msg_ring_fd
	op_cmd_acquire_br
	op_cmd_close_br
)

type Operation struct {
	code     uint8
	cmd      uint8
	flags    uint8
	sqeFlags uint8
	link     *Operation
	status   atomic.Int64
	resultCh chan Result // todo remove
	promise  Promise
	fd       int
	addr     unsafe.Pointer
	addrLen  uint32
	addr2    unsafe.Pointer
	addr2Len uint32
}

// Await result, can not use for multishot.
func (op *Operation) Await() (n int, cqeFlags uint32, err error) { // todo remove
	if op.flags&op_f_multishot != 0 {
		panic("op_f_multishot not supported")
		return
	}
	result, ok := <-op.resultCh
	if !ok {
		op.Close()
		err = ErrCanceled
		return
	}
	n, cqeFlags, err = result.N, result.Flags, result.Err
	if err != nil {
		if errors.Is(err, syscall.ECANCELED) {
			err = ErrCanceled
		}
		if op.flags&op_f_timeout != 0 {
			goto TIMEOUT
		}
		return
	}
MORE:
	if cqeFlags&liburing.IORING_CQE_F_MORE != 0 { // has more cqe (send_zc or sendmsg_zc)
		result, ok = <-op.resultCh
		if !ok {
			op.Close()
			err = ErrCanceled
			return
		}
		if result.Flags&liburing.IORING_CQE_F_NOTIF == 0 {
			goto MORE
		}
	}
TIMEOUT:
	if op.flags&op_f_timeout != 0 { // await op_f_timeout
		result, ok = <-op.link.resultCh
		if ok {
			if errors.Is(result.Err, syscall.ETIME) {
				err = ErrTimeout
			}
		}
	}
	return
}

func (op *Operation) Close() { // todo remove
	op.status.Store(CompletedOperationStatus)
	op.flags |= op_f_discard
}

func (op *Operation) Hijack() { // todo remove
	op.status.Store(HijackedOperationStatus)
}

func (op *Operation) Complete() { // todo remove (it will complete by canceled)
	op.status.Store(CompletedOperationStatus)
}

func (op *Operation) WithLinkTimeout(link *Operation) *Operation { // todo remove
	if link == nil {
		return op
	}
	op.flags |= op_f_timeout
	op.link = link
	op.sqeFlags |= liburing.IOSQE_IO_LINK
	return op
}

func (op *Operation) WithDeadline(resource *Resource, deadline time.Time) *Operation { // todo remove
	if deadline.IsZero() {
		return op
	}
	link := resource.AcquireOperation()
	link.PrepareLinkTimeout(deadline)
	op.WithLinkTimeout(link)
	return op
}

func (op *Operation) WithDirectAlloc(direct bool) *Operation {
	if direct {
		op.flags |= op_f_direct_alloc
	}
	return op
}

func (op *Operation) WithMultiShot() *Operation { // todo remove
	op.flags |= op_f_multishot
	return op
}

func (op *Operation) WithHandler(handler OperationHandler) *Operation { // todo remove
	if handler == nil {
		return op
	}
	op.flags |= op_f_with_handler
	op.addr2 = unsafe.Pointer(&handler)
	return op
}

func (op *Operation) handler() (handler OperationHandler) { // todo remove
	if op.flags&op_f_with_handler != 0 {
		handler = *(*OperationHandler)(op.addr2)
	}
	return
}

func (op *Operation) reset() {
	op.code = liburing.IORING_OP_LAST
	op.cmd = 0
	if op.flags&op_f_borrowed != 0 {
		op.flags = op_f_borrowed
	} else {
		op.flags = 0
	}
	op.sqeFlags = 0
	op.link = nil
	op.status.Store(ReadyOperationStatus)
	op.promise = nil

	op.fd = -1
	op.addr = nil
	op.addrLen = 0
	op.addr2 = nil
	op.addr2Len = 0
	return
}

func (op *Operation) failed(err error) {
	if op.status.CompareAndSwap(ReadyOperationStatus, CompletedOperationStatus) {
		op.handle(0, 0, err)
		return
	}
	if op.status.CompareAndSwap(ProcessingOperationStatus, CompletedOperationStatus) {
		op.handle(0, 0, err)
		return
	}
	if op.status.Load() == HijackedOperationStatus {
		op.handle(0, 0, err)
		return
	}
}

func (op *Operation) complete(n int, flags uint32, err error) {
	if ok := op.status.CompareAndSwap(ProcessingOperationStatus, CompletedOperationStatus); ok {
		op.handle(n, flags, err)
		return
	}
	if ok := op.status.Load() == HijackedOperationStatus; ok {
		op.handle(n, flags, err)
		return
	}
	return
}

func (op *Operation) handle(n int, flags uint32, err error) {
	if handler := op.handler(); handler != nil {
		handler.Handle(n, flags, err)
		return
	}
	if op.resultCh != nil {
		op.resultCh <- Result{n, flags, err}
	}
	return
}

func (op *Operation) completed() bool {
	return op.status.Load() == CompletedOperationStatus && len(op.resultCh) == 0
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
	ok := op.flags&op_f_borrowed != 0 && op.flags&op_f_discard == 0
	if ok {
		ok = op.clean()
	}
	return ok
}

func (op *Operation) clean() bool {
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
