//go:build linux

package aio

import (
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

func Recv(fd NetFd, b []byte, cb OperationCallback) {
	// op
	op := ReadOperator(fd)
	// check buf
	bLen := len(b)
	if bLen == 0 {
		cb(0, op.userdata, ErrEmptyBytes)
		return
	} else if bLen > MaxRW {
		b = b[:MaxRW]
	}
	// msg
	buf := op.userdata.Msg.Append(b)
	bufAddr := uintptr(unsafe.Pointer(buf.Base))
	bufLen := uint32(buf.Len)

	// cb
	op.callback = cb
	// completion
	op.completion = func(result int, cop *Operator, err error) {
		completeRecv(result, cop, err)
		runtime.KeepAlive(op)
	}
	// cylinder
	cylinder := nextIOURingCylinder()

	// timeout
	if timeout := op.timeout; timeout > 0 {
		timer := getOperatorTimer()
		op.timer = timer
		timer.Start(timeout, &operatorCanceler{
			cylinder: cylinder,
			op:       op,
		})
	}

	// prepare
	err := cylinder.prepare(opRecv, fd.Fd(), bufAddr, bufLen, 0, 0, op)
	runtime.KeepAlive(op)
	if err != nil {
		cb(0, op.userdata, os.NewSyscallError("io_uring_prep_recv", err))
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
	op := ReadOperator(fd)
	// check buf
	bLen := len(b)
	if bLen == 0 {
		cb(0, op.userdata, ErrEmptyBytes)
		return
	} else if bLen > MaxRW {
		b = b[:MaxRW]
	}
	// msg
	op.userdata.Msg.BuildRawSockaddrAny()
	op.userdata.Msg.Append(b)
	op.userdata.Msg.SetControl(oob)
	if fd.Family() == syscall.AF_UNIX {
		op.userdata.Msg.SetFlags(readMsgFlags)
	}

	// cb
	op.callback = cb
	// completion
	op.completion = func(result int, cop *Operator, err error) {
		completeRecvMsg(result, cop, err)
		runtime.KeepAlive(op)
	}
	// cylinder
	cylinder := nextIOURingCylinder()

	// timeout
	if timeout := op.timeout; timeout > 0 {
		timer := getOperatorTimer()
		op.timer = timer
		timer.Start(timeout, &operatorCanceler{
			cylinder: cylinder,
			op:       op,
		})
	}
	// prepare
	err := cylinder.prepare(opRecvmsg, fd.Fd(), uintptr(unsafe.Pointer(&op.userdata.Msg)), uint32(op.userdata.Msg.Iovlen), 0, 0, op)
	runtime.KeepAlive(op)
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
	runtime.KeepAlive(op.userdata)
	return
}
