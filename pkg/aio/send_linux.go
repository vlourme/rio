//go:build linux

package aio

import (
	"net"
	"os"
	"runtime"
	"unsafe"
)

func Send(fd NetFd, b []byte, cb OperationCallback) {
	// op
	op := WriteOperator(fd)
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
		completeSend(result, cop, err)
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
	err := cylinder.prepare(opSend, fd.Fd(), bufAddr, bufLen, 0, 0, op)
	runtime.KeepAlive(op)
	if err != nil {
		cb(0, op.userdata, os.NewSyscallError("io_uring_prep_send", err))
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

func completeSend(result int, op *Operator, err error) {
	if err != nil {
		err = os.NewSyscallError("io_uring_prep_send", err)
	}
	op.callback(result, op.userdata, err)
	return
}

func SendTo(fd NetFd, b []byte, addr net.Addr, cb OperationCallback) {
	SendMsg(fd, b, nil, addr, cb)
}

func SendMsg(fd NetFd, b []byte, oob []byte, addr net.Addr, cb OperationCallback) {
	// op
	op := WriteOperator(fd)
	// check buf
	bLen := len(b)
	if bLen == 0 {
		cb(0, op.userdata, ErrEmptyBytes)
		return
	}
	// msg
	_ = op.userdata.Msg.Append(b)
	op.userdata.Msg.SetControl(oob)
	_, saErr := op.userdata.Msg.SetAddr(addr)
	if saErr != nil {
		cb(0, op.userdata, saErr)
		return
	}

	// cb
	op.callback = cb
	// completion
	op.completion = func(result int, cop *Operator, err error) {
		completeSendMsg(result, cop, err)
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
	err := cylinder.prepare(opSendmsg, fd.Fd(), uintptr(unsafe.Pointer(&op.userdata.Msg)), 1, 0, 0, op)
	runtime.KeepAlive(op)
	if err != nil {
		cb(0, op.userdata, os.NewSyscallError("io_uring_prep_sendmsg", err))
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

func completeSendMsg(result int, op *Operator, err error) {
	if err != nil {
		err = os.NewSyscallError("io_uring_prep_sendmsg", err)
	}
	op.callback(result, op.userdata, err)
	return
}
