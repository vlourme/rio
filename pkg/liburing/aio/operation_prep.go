//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"syscall"
	"time"
	"unsafe"
)

func (op *Operation) PrepareNop() {
	op.code = liburing.IORING_OP_NOP
	return
}

func (op *Operation) PrepareCloseRing(key uint64) {
	op.code = liburing.IORING_OP_NOP
	op.cmd = op_cmd_close
	op.fd = int(key)
	return
}

func (op *Operation) packingNop(sqe *liburing.SubmissionQueueEntry) (err error) {
	switch op.cmd {
	case op_cmd_close: // close ring
		sqe.PrepareNop()
		sqe.SetData64(uint64(op.fd))
		break
	default:
		sqe.PrepareNop()
		break
	}
	return
}

func (op *Operation) PrepareLinkTimeout(deadline time.Time) {
	timeout := time.Until(deadline)
	if timeout < 1 {
		timeout = 10 * time.Microsecond
	}
	ts := syscall.NsecToTimespec(timeout.Nanoseconds())
	op.code = liburing.IORING_OP_LINK_TIMEOUT
	op.status.Store(ProcessingOperationStatus)
	op.addr = unsafe.Pointer(&ts)
}

func (op *Operation) packingLinkTimeout(sqe *liburing.SubmissionQueueEntry) (err error) {
	if op.addr == nil {
		return NewInvalidOpErr(errors.New("invalid link_timeout"))
	}
	ts := (*syscall.Timespec)(op.addr)
	sqe.PrepareLinkTimeout(ts, 0)
	sqe.SetData(unsafe.Pointer(op))
	return
}

func (op *Operation) PrepareMSGRing(ringFd int, n uint32) {
	op.code = liburing.IORING_OP_MSG_RING
	op.cmd = op_cmd_msg_ring
	op.fd = ringFd
	op.addrLen = n
	return
}

func (op *Operation) PrepareMSGRingFd(ringFd int, sourceFd int, targetOp *Operation) {
	op.code = liburing.IORING_OP_MSG_RING
	op.cmd = op_cmd_msg_ring_fd
	op.fd = ringFd
	op.addr = unsafe.Pointer(uintptr(sourceFd))
	op.addr2 = unsafe.Pointer(targetOp)
	return
}

func (op *Operation) packingMSGRing(sqe *liburing.SubmissionQueueEntry) (err error) {
	switch op.cmd {
	case op_cmd_msg_ring:
		fd := op.fd
		length := op.addrLen
		sqe.PrepareMsgRing(fd, length, nil, 0)
		break
	case op_cmd_msg_ring_fd:
		fd := op.fd
		sourceFd := int(uintptr(op.addr))
		userdata := op.addr2
		sqe.PrepareMsgRingFdAlloc(fd, sourceFd, userdata, 0)
		break
	default:
		err = NewInvalidOpErr(errors.New("invalid cmd"))
		return
	}
	sqe.SetData(unsafe.Pointer(op))
	return
}
