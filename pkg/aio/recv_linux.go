//go:build linux

package aio

import (
	"io"
	"os"
	"syscall"
	"unsafe"
)

func Recv(fd NetFd, b []byte, cb OperationCallback) {
	// op
	op := acquireOperator(fd)
	// cb
	op.callback = cb
	// completion
	op.completion = completeRecv
	// cylinder
	cylinder := nextIOURingCylinder()
	op.setCylinder(cylinder)

	// msg
	bufAddr := uintptr(unsafe.Pointer(&b[0]))
	bufLen := uint32(len(b))

	// prepare
	err := cylinder.prepareRW(opRecv, fd.Fd(), bufAddr, bufLen, 0, 0, op.ptr())
	if err != nil {
		cb(Userdata{}, os.NewSyscallError("io_uring_prep_recv", err))
		releaseOperator(op)
		return
	}
	return
}

func completeRecv(result int, op *Operator, err error) {
	cb := op.callback
	releaseOperator(op)
	if err != nil {
		err = os.NewSyscallError("io_uring_prep_recv", err)
		cb(Userdata{}, err)
		return
	}
	if result == 0 && op.fd.ZeroReadIsEOF() {
		cb(Userdata{}, io.EOF)
		return
	}
	cb(Userdata{N: result}, nil)
	return
}

func RecvFrom(fd NetFd, b []byte, cb OperationCallback) {
	RecvMsg(fd, b, nil, cb)
	return
}

func RecvMsg(fd NetFd, b []byte, oob []byte, cb OperationCallback) {
	// op
	op := acquireOperator(fd)
	// cb
	op.callback = cb
	// completion
	op.completion = completeRecvMsg
	// cylinder
	cylinder := nextIOURingCylinder()
	op.setCylinder(cylinder)
	// msg
	op.msg.Name = (*byte)(unsafe.Pointer(new(syscall.RawSockaddrAny)))
	op.msg.Namelen = syscall.SizeofSockaddrAny

	bLen := len(b)
	if bLen > 0 {
		op.msg.Iov = &syscall.Iovec{
			Base: &b[0],
			Len:  uint64(bLen),
		}
		op.msg.Iovlen = 1
	}
	if oobLen := len(oob); oobLen > 0 {
		op.msg.Control = &oob[0]
		op.msg.Controllen = uint64(oobLen)
		if bLen == 0 && fd.SocketType() != syscall.SOCK_DGRAM {
			var dummy byte
			op.msg.Iov = &syscall.Iovec{
				Base: &dummy,
				Len:  uint64(1),
			}
			op.msg.Iovlen = 1
		}
	}
	if fd.Family() == syscall.AF_UNIX {
		op.msg.Flags |= readMsgFlags
	}

	// prepare
	err := cylinder.prepareRW(opRecvmsg, fd.Fd(), uintptr(unsafe.Pointer(&op.msg)), uint32(op.msg.Iovlen), 0, 0, op.ptr())
	if err != nil {
		cb(Userdata{}, err)
		releaseOperator(op)
		return
	}
	return
}

func completeRecvMsg(result int, op *Operator, err error) {
	cb := op.callback
	msg := op.msg
	releaseOperator(op)
	if err != nil {
		err = os.NewSyscallError("io_uring_prep_recvmsg", err)
		cb(Userdata{}, err)
		return
	}
	rsa := (*syscall.RawSockaddrAny)(unsafe.Pointer(msg.Name))
	addr, addrErr := RawToAddr(rsa)
	if addrErr != nil {
		cb(Userdata{}, addrErr)
		return
	}
	oobn := int(msg.Controllen)
	flags := int(msg.Flags)
	cb(Userdata{N: result, OOBN: oobn, Addr: addr, MessageFlags: flags}, nil)
	return
}
