//go:build linux

package aio

import (
	"errors"
	"github.com/brickingsoft/rio/pkg/iouring"
	"syscall"
	"unsafe"
)

func (op *Operation) PrepareClose(fd int) {
	op.code = iouring.OpClose
	op.fd = fd
}

func (op *Operation) PrepareCloseDirect(filedIndex int) {
	op.code = iouring.OpClose
	op.fd = filedIndex
	if op.flags&directFd == 0 {
		op.flags |= directFd
	}
}

func (op *Operation) packingClose(sqe *iouring.SubmissionQueueEntry) (err error) {
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
		op.sqeFlags |= iouring.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= iouring.SQEAsync
	}
	op.code = iouring.OpShutdown
	op.fd = fd
	op.addr2 = unsafe.Pointer(uintptr(syscall.SHUT_RD))
	return
}

func (op *Operation) PrepareCloseWrite(nfd *NetFd) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= iouring.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= iouring.SQEAsync
	}
	op.code = iouring.OpShutdown
	op.fd = fd
	op.addr2 = unsafe.Pointer(uintptr(syscall.SHUT_WR))
	return
}

func (op *Operation) packingShutdown(sqe *iouring.SubmissionQueueEntry) (err error) {
	cmd := int(uintptr(op.addr2))
	sqe.PrepareShutdown(op.fd, cmd)
	sqe.SetFlags(op.sqeFlags)
	sqe.SetData(unsafe.Pointer(op))
	return
}

func (op *Operation) PrepareCancel(target *Operation) {
	op.code = iouring.OpAsyncCancel
	op.addr2 = unsafe.Pointer(target)
}

func (op *Operation) PrepareCancelFd(fd int) {
	op.code = iouring.OpAsyncCancel
	op.fd = fd
}

func (op *Operation) PrepareCancelFixedFd(fileIndex int) {
	op.code = iouring.OpAsyncCancel
	op.fd = fileIndex
	op.flags |= directFd
}

func (op *Operation) packingCancel(sqe *iouring.SubmissionQueueEntry) (err error) {
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
	op.code = iouring.OPFixedFdInstall
	op.fd = fd
}

func (op *Operation) packingFixedFdInstall(sqe *iouring.SubmissionQueueEntry) (err error) {
	sqe.PrepareFixedFdInstall(op.fd, 0)
	sqe.SetData(unsafe.Pointer(op))
	return err
}
