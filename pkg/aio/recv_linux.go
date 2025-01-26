//go:build linux

package aio

import (
	"io"
	"os"
	"syscall"
	"unsafe"
)

func Recv(fd NetFd, b []byte, cb OperationCallback) {
	// check buf
	bLen := len(b)
	if bLen == 0 {
		cb(Userdata{}, ErrEmptyBytes)
		return
	} else if bLen > MaxRW {
		b = b[:MaxRW]
	}
	// op
	op := fd.ReadOperator()
	// msg
	buf := op.msg.Append(b)
	bufAddr := uintptr(unsafe.Pointer(buf.Base))
	bufLen := uint32(buf.Len)

	// cb
	op.callback = cb
	// completion
	op.completion = completeRecv
	// cylinder
	cylinder := nextIOURingCylinder()

	// timeout
	op.tryPrepareTimeout(cylinder)

	// prepare
	err := cylinder.prepareRW(opRecv, fd.Fd(), bufAddr, bufLen, 0, 0, op.ptr())
	if err != nil {
		cb(Userdata{}, os.NewSyscallError("io_uring_prep_recv", err))
		op.reset()
		return
	}
	return
}

func completeRecv(result int, op *Operator, err error) {
	if err != nil {
		err = os.NewSyscallError("io_uring_prep_recv", err)
		op.callback(Userdata{}, err)
		return
	}
	if result == 0 && op.fd.ZeroReadIsEOF() {
		op.callback(Userdata{}, io.EOF)
		return
	}
	op.callback(Userdata{QTY: result, Msg: op.msg}, nil)
	return
}

func RecvFrom(fd NetFd, b []byte, cb OperationCallback) {
	RecvMsg(fd, b, nil, cb)
	return
}

func RecvMsg(fd NetFd, b []byte, oob []byte, cb OperationCallback) {
	// check buf
	bLen := len(b)
	if bLen == 0 {
		cb(Userdata{}, ErrEmptyBytes)
		return
	}
	// op
	op := fd.ReadOperator()
	// msg
	op.msg.BuildRawSockaddrAny()
	op.msg.Append(b)
	op.msg.SetControl(oob)
	if fd.Family() == syscall.AF_UNIX {
		op.msg.SetFlags(readMsgFlags)
	}

	// cb
	op.callback = cb
	// completion
	op.completion = completeRecvMsg
	// cylinder
	cylinder := nextIOURingCylinder()

	// timeout
	op.tryPrepareTimeout(cylinder)

	// prepare
	err := cylinder.prepareRW(opRecvmsg, fd.Fd(), uintptr(unsafe.Pointer(&op.msg)), uint32(op.msg.Iovlen), 0, 0, op.ptr())
	if err != nil {
		cb(Userdata{}, err)
		op.reset()
		return
	}
	return
}

func completeRecvMsg(result int, op *Operator, err error) {
	if err != nil {
		err = os.NewSyscallError("io_uring_prep_recvmsg", err)
		op.callback(Userdata{}, err)
		return
	}
	op.callback(Userdata{QTY: result, Msg: op.msg}, nil)
	return
}
