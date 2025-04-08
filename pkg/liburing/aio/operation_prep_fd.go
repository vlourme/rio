//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"syscall"
	"unsafe"
)

func (op *Operation) PrepareClose(fd int) {
	op.code = liburing.IORING_OP_CLOSE
	op.fd = fd
}

func (op *Operation) PrepareCloseDirect(filedIndex int) {
	op.code = liburing.IORING_OP_CLOSE
	op.fd = filedIndex
	if op.flags&op_f_direct_alloc == 0 {
		op.flags |= op_f_direct_alloc
	}
}

func (op *Operation) packingClose(sqe *liburing.SubmissionQueueEntry) (err error) {
	if op.flags&op_f_direct_alloc != 0 {
		sqe.PrepareCloseDirect(uint32(op.fd))
	} else {
		sqe.PrepareClose(op.fd)
	}
	sqe.SetData(unsafe.Pointer(op))
	return
}

func (op *Operation) PrepareCloseRead(nfd *Conn) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.IOSQE_FIXED_FILE
	}
	op.code = liburing.IORING_OP_SHUTDOWN
	op.fd = fd
	op.addr2 = unsafe.Pointer(uintptr(syscall.SHUT_RD))
	return
}

func (op *Operation) PrepareCloseWrite(nfd *Conn) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.IOSQE_FIXED_FILE
	}
	op.code = liburing.IORING_OP_SHUTDOWN
	op.fd = fd
	op.addr2 = unsafe.Pointer(uintptr(syscall.SHUT_WR))
	return
}

func (op *Operation) packingShutdown(sqe *liburing.SubmissionQueueEntry) (err error) {
	cmd := int(uintptr(op.addr2))
	sqe.PrepareShutdown(op.fd, cmd)
	sqe.SetFlags(op.sqeFlags)
	sqe.SetData(unsafe.Pointer(op))
	return
}

func (op *Operation) PrepareCancel(target *Operation) {
	op.code = liburing.IORING_OP_ASYNC_CANCEL
	op.addr2 = unsafe.Pointer(target)
}

func (op *Operation) PrepareCancelFd(fd int) {
	op.code = liburing.IORING_OP_ASYNC_CANCEL
	op.fd = fd
}

func (op *Operation) PrepareCancelFixedFd(fileIndex int) {
	op.code = liburing.IORING_OP_ASYNC_CANCEL
	op.fd = fileIndex
	op.flags |= op_f_direct_alloc
}

func (op *Operation) packingCancel(sqe *liburing.SubmissionQueueEntry) (err error) {
	if op.fd > -1 { // cancel fd
		if op.flags&op_f_direct_alloc != 0 { // cancel direct
			sqe.PrepareCancelFdFixed(uint32(op.fd), 0)
		} else { // cancel regular
			sqe.PrepareCancelFd(op.fd, 0)
		}
	} else if op.addr2 != nil { // cancel op
		sqe.PrepareCancel(uintptr(op.addr2), 0)
	} else {
		err = NewInvalidOpErr(errors.New("invalid cancel params"))
		return
	}
	sqe.SetData(unsafe.Pointer(op))
	return
}

func (op *Operation) PrepareFixedFdInstall(fd int) {
	op.code = liburing.IORING_OP_FIXED_FD_INSTALL
	op.fd = fd
}

func (op *Operation) packingFixedFdInstall(sqe *liburing.SubmissionQueueEntry) (err error) {
	sqe.PrepareFixedFdInstall(op.fd, 0)
	sqe.SetData(unsafe.Pointer(op))
	return err
}
