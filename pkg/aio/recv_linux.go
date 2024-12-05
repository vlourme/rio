//go:build linux

package aio

import (
	"os"
	"syscall"
	"unsafe"
)

func Recv(fd NetFd, b []byte, cb OperationCallback) {
	// op
	op := fd.ReadOperator()
	// check buf
	bLen := len(b)
	if bLen == 0 {
		cb(0, op.userdata, ErrEmptyBytes)
		return
	} else if bLen > MaxRW {
		b = b[:MaxRW]
	}
	// msg
	msg := HDRMessage{}
	buf := msg.Append(b)
	bufAddr := uintptr(unsafe.Pointer(buf.Base))
	bufLen := uint32(buf.Len)
	op.userdata.Msg = &msg

	// cb
	op.callback = cb
	// completion
	op.completion = completeRecv
	// cylinder
	cylinder := nextIOURingCylinder()

	// userdata
	userdata := uint64(uintptr(unsafe.Pointer(&op)))
	// timeout
	if timeout := op.timeout; timeout > 0 {
		timer := getOperatorTimer()
		op.timer = timer
		timer.Start(timeout, &operatorCanceler{
			cylinder: cylinder,
			op:       &op,
		})
	}

	// prepare
	err := cylinder.prepare(opRecv, fd.Fd(), bufAddr, bufLen, 0, 0, userdata)
	if err != nil {
		cb(0, op.userdata, err)
		// reset
		op.callback = nil
		op.completion = nil
		if op.timer != nil {
			timer := op.timer
			timer.Done()
			putOperatorTimer(timer)
			op.timer = nil
		}
		return
	}
	return
}

func completeRecv(result int, op *Operator, err error) {
	if err != nil {
		err = os.NewSyscallError("io_uring_prep_recv", err)
	}
	op.userdata.QTY = uint32(result)
	op.callback(result, op.userdata, eofError(op.fd, result, err))
	return
}

func RecvFrom(fd NetFd, b []byte, cb OperationCallback) {
	RecvMsg(fd, b, nil, cb)
	return
}

func RecvMsg(fd NetFd, b []byte, oob []byte, cb OperationCallback) {
	// op
	op := fd.ReadOperator()
	// check buf
	bLen := len(b)
	if bLen == 0 {
		cb(0, op.userdata, ErrEmptyBytes)
		return
	} else if bLen > MaxRW {
		b = b[:MaxRW]
	}
	// msg
	msg := HDRMessage{}
	msg.BuildRawSockaddrAny()
	msg.Append(b)
	msg.SetControl(oob)
	if fd.Family() == syscall.AF_UNIX {
		msg.SetFlags(readMsgFlags)
	}
	op.userdata.Msg = &msg

	// cb
	op.callback = cb
	// completion
	op.completion = completeRecvMsg
	// cylinder
	cylinder := nextIOURingCylinder()

	// userdata
	userdata := uint64(uintptr(unsafe.Pointer(&op)))
	// timeout
	if timeout := op.timeout; timeout > 0 {
		timer := getOperatorTimer()
		op.timer = timer
		timer.Start(timeout, &operatorCanceler{
			cylinder: cylinder,
			op:       &op,
		})
	}
	// prepare
	err := cylinder.prepare(opRecvmsg, fd.Fd(), uintptr(unsafe.Pointer(&msg)), uint32(msg.Iovlen), 0, 0, userdata)
	if err != nil {
		cb(0, op.userdata, err)
		// reset
		op.callback = nil
		op.completion = nil
		if op.timer != nil {
			timer := op.timer
			timer.Done()
			putOperatorTimer(timer)
			op.timer = nil
		}
		return
	}
	return
}

func completeRecvMsg(result int, op *Operator, err error) {
	if err != nil {
		err = os.NewSyscallError("io_uring_prep_recvmsg", err)
	}
	op.userdata.QTY = uint32(result)
	op.callback(result, op.userdata, err)
	return
}
