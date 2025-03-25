//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/liburing"
	"syscall"
	"unsafe"
)

func (op *Operation) PrepareConnect(nfd *NetFd, addr *syscall.RawSockaddrAny, addrLen int) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= liburing.SQEAsync
	}
	op.code = liburing.OpConnect
	op.fd = fd
	op.addr = unsafe.Pointer(addr)
	op.addrLen = uint32(addrLen)
	return
}

func (op *Operation) packingConnect(sqe *liburing.SubmissionQueueEntry) (err error) {
	addrPtr := (*syscall.RawSockaddrAny)(op.addr)
	addrLenPtr := uint64(op.addrLen)
	sqe.PrepareConnect(op.fd, addrPtr, addrLenPtr)
	sqe.SetFlags(op.sqeFlags)
	sqe.SetData(unsafe.Pointer(op))
	return
}

func (op *Operation) PrepareListen(nfd *NetFd, backlog int) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= liburing.SQEAsync
	}
	op.code = liburing.OpListen
	op.fd = fd
	op.addrLen = uint32(backlog)
	return
}

func (op *Operation) packingListen(sqe *liburing.SubmissionQueueEntry) (err error) {
	sqe.PrepareListen(op.fd, op.addrLen)
	sqe.SetFlags(op.sqeFlags)
	sqe.SetData(unsafe.Pointer(op))
	return
}

func (op *Operation) PrepareBind(nfd *NetFd, addr *syscall.RawSockaddrAny, addrLen int) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= liburing.SQEAsync
	}
	op.code = liburing.OpBind
	op.fd = fd
	op.addr = unsafe.Pointer(addr)
	op.addrLen = uint32(addrLen)
	return
}

func (op *Operation) packingBind(sqe *liburing.SubmissionQueueEntry) (err error) {
	addrPtr := (*syscall.RawSockaddrAny)(op.addr)
	addrLenPtr := uint64(op.addrLen)
	sqe.PrepareBind(op.fd, addrPtr, addrLenPtr)
	sqe.SetFlags(op.sqeFlags)
	sqe.SetData(unsafe.Pointer(op))
	return
}

func (op *Operation) PrepareAccept(ln *NetFd, addr *syscall.RawSockaddrAny, addrLen *int) {
	fd, direct := ln.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.SQEFixedFile
	}
	if ln.Async() {
		op.sqeFlags |= liburing.SQEAsync
	}
	op.code = liburing.OpAccept
	op.fd = fd
	op.addr = unsafe.Pointer(addr)
	op.addr2 = unsafe.Pointer(addrLen)
	if op.flags&directFd != 0 {
		op.addrLen = uint32(syscall.SOCK_NONBLOCK)
	} else {
		op.addrLen = uint32(syscall.SOCK_NONBLOCK | syscall.SOCK_CLOEXEC)
	}
	return
}

func (op *Operation) PrepareAcceptMultishot(ln *NetFd, addr *syscall.RawSockaddrAny, addrLen *int) {
	fd, direct := ln.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.SQEFixedFile
	}
	if ln.Async() {
		op.sqeFlags |= liburing.SQEAsync
	}

	op.code = liburing.OpAccept
	op.fd = fd
	op.flags |= multishot
	op.addr = unsafe.Pointer(addr)
	op.addr2 = unsafe.Pointer(addrLen)
	if op.flags&directFd != 0 {
		op.addrLen = uint32(syscall.SOCK_NONBLOCK)
	} else {
		op.addrLen = uint32(syscall.SOCK_NONBLOCK | syscall.SOCK_CLOEXEC)
	}
	return
}

func (op *Operation) packingAccept(sqe *liburing.SubmissionQueueEntry) (err error) {
	addrPtr := (*syscall.RawSockaddrAny)(op.addr)
	addrLenPtr := uint64(uintptr(op.addr2))
	flags := int(op.addrLen)
	if op.flags&multishot != 0 {
		if op.flags&directFd != 0 {
			sqe.PrepareAcceptMultishotDirect(op.fd, addrPtr, addrLenPtr, flags)
		} else {
			sqe.PrepareAcceptMultishot(op.fd, addrPtr, addrLenPtr, flags)
		}
	} else {
		if op.flags&directFd != 0 {
			sqe.PrepareAcceptDirect(op.fd, addrPtr, addrLenPtr, flags, liburing.FileIndexAlloc)
		} else {
			sqe.PrepareAccept(op.fd, addrPtr, addrLenPtr, flags)
		}
	}
	sqe.SetFlags(op.sqeFlags)
	sqe.SetData(unsafe.Pointer(op))
	return
}

func (op *Operation) PrepareReceive(nfd *NetFd, b []byte) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= liburing.SQEAsync
	}
	op.code = liburing.OpRecv
	op.fd = fd
	op.addr = unsafe.Pointer(&b[0])
	op.addrLen = uint32(len(b))
	return
}

func (op *Operation) packingReceive(sqe *liburing.SubmissionQueueEntry) (err error) {
	b := uintptr(op.addr)
	bLen := op.addrLen
	sqe.PrepareRecv(op.fd, b, bLen, 0)
	sqe.SetFlags(op.sqeFlags)
	sqe.SetData(unsafe.Pointer(op))
	return
}

func (op *Operation) PrepareSend(nfd *NetFd, b []byte) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= liburing.SQEAsync
	}
	op.code = liburing.OpSend
	op.fd = fd
	op.addr = unsafe.Pointer(&b[0])
	op.addrLen = uint32(len(b))
	return
}

func (op *Operation) packingSend(sqe *liburing.SubmissionQueueEntry) (err error) {
	b := uintptr(op.addr)
	bLen := op.addrLen
	sqe.PrepareSend(op.fd, b, bLen, 0)
	sqe.SetFlags(op.sqeFlags)
	sqe.SetData(unsafe.Pointer(op))
	return
}

func (op *Operation) PrepareSendZC(nfd *NetFd, b []byte) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= liburing.SQEAsync
	}
	op.code = liburing.OpSendZC
	op.fd = fd
	op.addr = unsafe.Pointer(&b[0])
	op.addrLen = uint32(len(b))
	return
}

func (op *Operation) packingSendZC(sqe *liburing.SubmissionQueueEntry) (err error) {
	b := uintptr(op.addr)
	bLen := op.addrLen
	sqe.PrepareSendZC(op.fd, b, bLen, 0, 0)
	sqe.SetFlags(op.sqeFlags)
	sqe.SetData(unsafe.Pointer(op))
	return
}

func (op *Operation) PrepareReceiveMsg(nfd *NetFd, msg *syscall.Msghdr) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= liburing.SQEAsync
	}
	op.code = liburing.OpRecvmsg
	op.fd = fd
	op.addr = unsafe.Pointer(msg)
	return
}

func (op *Operation) packingReceiveMsg(sqe *liburing.SubmissionQueueEntry) (err error) {
	msg := (*syscall.Msghdr)(op.addr)
	sqe.PrepareRecvMsg(op.fd, msg, 0)
	sqe.SetFlags(op.sqeFlags)
	sqe.SetData(unsafe.Pointer(op))
	return
}

func (op *Operation) PrepareSendMsg(nfd *NetFd, msg *syscall.Msghdr) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= liburing.SQEAsync
	}
	op.code = liburing.OpSendmsg
	op.fd = fd
	op.addr = unsafe.Pointer(msg)
	return
}

func (op *Operation) packingSendMsg(sqe *liburing.SubmissionQueueEntry) (err error) {
	msg := (*syscall.Msghdr)(op.addr)
	sqe.PrepareSendMsg(op.fd, msg, 0)
	sqe.SetFlags(op.sqeFlags)
	sqe.SetData(unsafe.Pointer(op))
	return
}

func (op *Operation) PrepareSendMsgZC(nfd *NetFd, msg *syscall.Msghdr) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= liburing.SQEAsync
	}
	op.code = liburing.OpSendMsgZC
	op.fd = fd
	op.addr = unsafe.Pointer(msg)
	return
}

func (op *Operation) packingSendMsgZc(sqe *liburing.SubmissionQueueEntry) (err error) {
	msg := (*syscall.Msghdr)(op.addr)
	sqe.PrepareSendmsgZC(op.fd, msg, 0)
	sqe.SetFlags(op.sqeFlags)
	sqe.SetData(unsafe.Pointer(op))
	return
}
