//go:build linux

package aio

import (
	"net"
	"os"
	"unsafe"
)

// Send
// send_zc: available since 6.0
// sendmsg_zc: available since 6.1
// https://manpages.debian.org/unstable/liburing-dev/io_uring_enter.2.en.html
func Send(fd NetFd, b []byte, cb OperationCallback) {
	// check buf
	bLen := len(b)
	if bLen == 0 {
		cb(Userdata{}, ErrEmptyBytes)
		return
	} else if bLen > MaxRW {
		b = b[:MaxRW]
	}
	op := fd.WriteOperator()
	// msg
	buf := op.msg.Append(b)
	bufAddr := uintptr(unsafe.Pointer(buf.Base))
	bufLen := uint32(buf.Len)

	// cb
	op.callback = cb
	// completion
	op.completion = completeSend

	// cylinder
	cylinder := nextIOURingCylinder()

	// timeout
	op.tryPrepareTimeout(cylinder)

	// prepare
	err := cylinder.prepare(opSend, fd.Fd(), bufAddr, bufLen, 0, 0, op)
	if err != nil {
		cb(Userdata{}, os.NewSyscallError("io_uring_prep_send", err))
		op.clean()
	}
	return
}

func completeSend(result int, op *Operator, err error) {
	if err != nil {
		err = os.NewSyscallError("io_uring_prep_send", err)
		op.callback(Userdata{}, err)
		return
	}
	op.callback(Userdata{QTY: result}, nil)
	return
}

func SendTo(fd NetFd, b []byte, addr net.Addr, cb OperationCallback) {
	SendMsg(fd, b, nil, addr, cb)
}

func SendMsg(fd NetFd, b []byte, oob []byte, addr net.Addr, cb OperationCallback) {
	// check buf
	bLen := len(b)
	if bLen == 0 {
		cb(Userdata{}, ErrEmptyBytes)
		return
	}
	// op
	op := fd.WriteOperator()
	// msg
	_ = op.msg.Append(b)
	op.msg.SetControl(oob)
	_, saErr := op.msg.SetAddr(addr)
	if saErr != nil {
		cb(Userdata{}, saErr)
		op.clean()
		return
	}

	// cb
	op.callback = cb
	// completion
	op.completion = completeSendMsg
	// cylinder
	cylinder := nextIOURingCylinder()

	// timeout
	op.tryPrepareTimeout(cylinder)

	// prepare
	err := cylinder.prepare(opSendmsg, fd.Fd(), uintptr(unsafe.Pointer(&op.msg)), 1, 0, 0, op)
	if err != nil {
		cb(Userdata{}, os.NewSyscallError("io_uring_prep_sendmsg", err))
		op.clean()
	}
	return
}

func completeSendMsg(result int, op *Operator, err error) {
	if err != nil {
		err = os.NewSyscallError("io_uring_prep_sendmsg", err)
		op.callback(Userdata{}, err)
		return
	}
	op.callback(Userdata{QTY: result, Msg: op.msg}, nil)
	return
}
