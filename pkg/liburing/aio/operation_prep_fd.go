//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/liburing"
	"syscall"
	"unsafe"
)

func (op *Operation) PrepareClose(fd int) {
	op.code = liburing.OpClose
	op.fd = fd
}

func (op *Operation) PrepareCloseDirect(filedIndex int) {
	op.code = liburing.OpClose
	op.fd = filedIndex
	if op.flags&directFd == 0 {
		op.flags |= directFd
	}
}

func (op *Operation) packingClose(sqe *liburing.SubmissionQueueEntry) (err error) {
	if op.flags&directFd != 0 {
		sqe.PrepareCloseDirect(uint32(op.fd))
	} else {
		sqe.PrepareClose(op.fd)
	}
	sqe.SetData(unsafe.Pointer(op))
	return
}

func (op *Operation) PrepareCloseRead(nfd *NetFd) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= liburing.SQEAsync
	}
	op.code = liburing.OpShutdown
	op.fd = fd
	op.addr2 = unsafe.Pointer(uintptr(syscall.SHUT_RD))
	return
}

func (op *Operation) PrepareCloseWrite(nfd *NetFd) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= liburing.SQEAsync
	}
	op.code = liburing.OpShutdown
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
	op.code = liburing.OpAsyncCancel
	op.addr2 = unsafe.Pointer(target)
}

func (op *Operation) PrepareCancelFd(fd int) {
	op.code = liburing.OpAsyncCancel
	op.fd = fd
}

func (op *Operation) PrepareCancelFixedFd(fileIndex int) {
	op.code = liburing.OpAsyncCancel
	op.fd = fileIndex
	op.flags |= directFd
}

func (op *Operation) packingCancel(sqe *liburing.SubmissionQueueEntry) (err error) {
	if op.fd > -1 { // cancel fd
		if op.flags&directFd != 0 { // cancel direct
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
	op.code = liburing.OPFixedFdInstall
	op.fd = fd
}

func (op *Operation) packingFixedFdInstall(sqe *liburing.SubmissionQueueEntry) (err error) {
	sqe.PrepareFixedFdInstall(op.fd, 0)
	sqe.SetData(unsafe.Pointer(op))
	return err
}
