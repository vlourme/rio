//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/liburing"
	"syscall"
	"unsafe"
)

func (op *Operation) PrepareConnect(nfd *Conn, addr *syscall.RawSockaddrAny, addrLen int) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.IOSQE_FIXED_FILE
	}
	op.code = liburing.IORING_OP_CONNECT
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

func (op *Operation) PrepareListen(nfd *Listener, backlog int) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.IOSQE_FIXED_FILE
	}
	op.code = liburing.IORING_OP_LISTEN
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
		op.sqeFlags |= liburing.IOSQE_FIXED_FILE
	}
	op.code = liburing.IORING_OP_BIND
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

type prepareAcceptParam struct {
	addr    *syscall.RawSockaddrAny
	addrLen *int
}

func (op *Operation) PrepareAccept(ln *Listener, param *prepareAcceptParam) {
	fd, direct := ln.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.IOSQE_FIXED_FILE
	}
	op.code = liburing.IORING_OP_ACCEPT
	op.fd = fd
	op.addr = unsafe.Pointer(param)
	return
}

func (op *Operation) PrepareAcceptMultishot(ln *Listener, param *prepareAcceptParam, handler OperationHandler) {
	fd, direct := ln.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.IOSQE_FIXED_FILE
	}
	op.code = liburing.IORING_OP_ACCEPT
	op.fd = fd
	op.addr = unsafe.Pointer(param)
	op.WithMultiShot().WithHandler(handler)
	return
}

func (op *Operation) packingAccept(sqe *liburing.SubmissionQueueEntry) (err error) {
	param := (*prepareAcceptParam)(op.addr)
	addrPtr := param.addr
	addrLenPtr := uint64(uintptr(unsafe.Pointer(param.addrLen)))
	if op.flags&multishot != 0 {
		if op.sqeFlags&liburing.IOSQE_FIXED_FILE != 0 {
			sqe.PrepareAcceptMultishotDirect(op.fd, addrPtr, addrLenPtr, syscall.SOCK_NONBLOCK)
		} else {
			sqe.PrepareAcceptMultishot(op.fd, addrPtr, addrLenPtr, syscall.SOCK_NONBLOCK|syscall.SOCK_CLOEXEC)
		}
	} else {
		if op.sqeFlags&liburing.IOSQE_FIXED_FILE != 0 {
			sqe.PrepareAcceptDirect(op.fd, addrPtr, addrLenPtr, syscall.SOCK_NONBLOCK, liburing.IORING_FILE_INDEX_ALLOC)
		} else {
			sqe.PrepareAccept(op.fd, addrPtr, addrLenPtr, syscall.SOCK_NONBLOCK|syscall.SOCK_CLOEXEC)
		}
	}
	sqe.SetFlags(op.sqeFlags)
	sqe.SetData(unsafe.Pointer(op))
	return
}

func (op *Operation) PrepareReceive(nfd *Conn, b []byte) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.IOSQE_FIXED_FILE
	}
	op.code = liburing.IORING_OP_RECV
	op.fd = fd
	op.addr = unsafe.Pointer(&b[0])
	op.addrLen = uint32(len(b))
	return
}

func (op *Operation) PrepareReceiveMultishot(nfd *Conn, br *BufferAndRing, handler OperationHandler) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.IOSQE_FIXED_FILE
	}
	op.sqeFlags |= liburing.IOSQE_BUFFER_SELECT
	op.code = liburing.IORING_OP_RECV
	op.fd = fd
	op.addrLen = uint32(br.bgid)
	op.WithMultiShot().WithHandler(handler)
	return
}

func (op *Operation) packingReceive(sqe *liburing.SubmissionQueueEntry) (err error) {
	if op.flags&multishot != 0 {
		bgid := uint16(op.addrLen)
		sqe.PrepareRecvMultishot(op.fd, 0, 0, 0)
		if liburing.VersionEnable(6, 10, 0) {
			sqe.SetIoPrio(liburing.IORING_RECVSEND_BUNDLE)
		}
		sqe.SetBufferGroup(bgid)
	} else {
		b := uintptr(op.addr)
		bLen := op.addrLen
		sqe.PrepareRecv(op.fd, b, bLen, 0)
	}
	sqe.SetFlags(op.sqeFlags)
	sqe.SetData(unsafe.Pointer(op))
	return
}

func (op *Operation) PrepareSend(nfd *Conn, b []byte) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.IOSQE_FIXED_FILE
	}
	op.code = liburing.IORING_OP_SEND
	op.fd = fd
	op.addr = unsafe.Pointer(&b[0])
	op.addrLen = uint32(len(b))
	return
}

func (op *Operation) packingSend(sqe *liburing.SubmissionQueueEntry) (err error) {
	b := uintptr(op.addr)
	bLen := op.addrLen
	flags := 0
	if op.sqeFlags&liburing.IOSQE_IO_LINK != 0 {
		flags = syscall.MSG_WAITALL
	}
	sqe.PrepareSend(op.fd, b, bLen, flags)
	sqe.SetFlags(op.sqeFlags)
	sqe.SetData(unsafe.Pointer(op))
	return
}

func (op *Operation) PrepareSendZC(nfd *Conn, b []byte) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.IOSQE_FIXED_FILE
	}
	op.code = liburing.IORING_OP_SEND_ZC
	op.fd = fd
	op.addr = unsafe.Pointer(&b[0])
	op.addrLen = uint32(len(b))
	return
}

func (op *Operation) packingSendZC(sqe *liburing.SubmissionQueueEntry) (err error) {
	b := uintptr(op.addr)
	bLen := op.addrLen
	flags := 0
	if op.sqeFlags&liburing.IOSQE_IO_LINK != 0 {
		flags = syscall.MSG_WAITALL
	}
	sqe.PrepareSendZC(op.fd, b, bLen, flags, 0)
	sqe.SetFlags(op.sqeFlags)
	sqe.SetData(unsafe.Pointer(op))
	return
}

func (op *Operation) PrepareReceiveMsg(nfd *Conn, msg *syscall.Msghdr) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.IOSQE_FIXED_FILE
	}
	op.code = liburing.IORING_OP_RECVMSG
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

func (op *Operation) PrepareSendMsg(nfd *Conn, msg *syscall.Msghdr) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.IOSQE_FIXED_FILE
	}
	op.code = liburing.IORING_OP_SENDMSG
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

func (op *Operation) PrepareSendMsgZC(nfd *Conn, msg *syscall.Msghdr) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= liburing.IOSQE_FIXED_FILE
	}
	op.code = liburing.IORING_OP_SENDMSG_ZC
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
