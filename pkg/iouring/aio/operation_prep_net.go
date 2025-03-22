//go:build linux

package aio

import (
	"github.com/brickingsoft/rio/pkg/iouring"
	"syscall"
	"unsafe"
)

func (op *Operation) PrepareConnect(nfd *NetFd, addr *syscall.RawSockaddrAny, addrLen int) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= iouring.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= iouring.SQEAsync
	}
	op.code = iouring.OpConnect
	op.fd = fd
	op.msg.Name = (*byte)(unsafe.Pointer(addr))
	op.msg.Namelen = uint32(addrLen)
	return
}

func (op *Operation) PrepareAccept(ln *NetFd, addr *syscall.RawSockaddrAny, addrLen int) {
	fd, direct := ln.FileDescriptor()
	if direct {
		op.sqeFlags |= iouring.SQEFixedFile
	}
	if ln.Async() {
		op.sqeFlags |= iouring.SQEAsync
	}
	op.code = iouring.OpAccept
	op.fd = fd
	op.msg.Name = (*byte)(unsafe.Pointer(addr))
	op.msg.Namelen = uint32(addrLen)
	return
}

func (op *Operation) PrepareAcceptMultishot(ln *NetFd, addr *syscall.RawSockaddrAny, addrLen int) {
	fd, direct := ln.FileDescriptor()
	if direct {
		op.sqeFlags |= iouring.SQEFixedFile
	}
	if ln.Async() {
		op.sqeFlags |= iouring.SQEAsync
	}
	op.pipe.fdIn = syscall.SOCK_NONBLOCK
	op.code = iouring.OpAccept
	op.multishot = true
	op.fd = fd
	op.msg.Name = (*byte)(unsafe.Pointer(addr))
	op.msg.Namelen = uint32(addrLen)
	return
}

func (op *Operation) PrepareReceive(nfd *NetFd, b []byte) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= iouring.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= iouring.SQEAsync
	}
	op.code = iouring.OpRecv
	op.fd = fd
	op.msg.Name = &b[0]
	op.msg.Namelen = uint32(len(b))
	return
}

func (op *Operation) PrepareSend(nfd *NetFd, b []byte) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= iouring.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= iouring.SQEAsync
	}
	op.code = iouring.OpSend
	op.fd = fd
	op.msg.Name = &b[0]
	op.msg.Namelen = uint32(len(b))
	return
}

func (op *Operation) PrepareSendZC(nfd *NetFd, b []byte) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= iouring.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= iouring.SQEAsync
	}
	op.code = iouring.OpSendZC
	op.fd = fd
	op.msg.Name = &b[0]
	op.msg.Namelen = uint32(len(b))
	return
}

func (op *Operation) PrepareReceiveMsg(nfd *NetFd, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= iouring.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= iouring.SQEAsync
	}
	op.code = iouring.OpRecvmsg
	op.fd = fd
	op.setMsg(b, oob, addr, addrLen, flags)
	return
}

func (op *Operation) PrepareSendMsg(nfd *NetFd, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= iouring.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= iouring.SQEAsync
	}
	op.code = iouring.OpSendmsg
	op.fd = fd
	op.setMsg(b, oob, addr, addrLen, flags)
	return
}

func (op *Operation) PrepareSendMsgZC(nfd *NetFd, b []byte, oob []byte, addr *syscall.RawSockaddrAny, addrLen int, flags int32) {
	fd, direct := nfd.FileDescriptor()
	if direct {
		op.sqeFlags |= iouring.SQEFixedFile
	}
	if nfd.Async() {
		op.sqeFlags |= iouring.SQEAsync
	}
	op.code = iouring.OpSendMsgZC
	op.fd = fd
	op.setMsg(b, oob, addr, addrLen, flags)
	return
}
