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
	op := fd.WriteOperator()
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
	op.completion = completeSend

	// cylinder
	cylinder := nextIOURingCylinder()

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
	err := cylinder.prepare(opSend, fd.Fd(), bufAddr, bufLen, 0, 0, &op)
	runtime.KeepAlive(&op)
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
	// op
	op := fd.WriteOperator()
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
	_, saErr := msg.SetAddr(addr)
	if saErr != nil {
		cb(0, op.userdata, saErr)
		return
	}

	op.userdata.Msg = &msg

	// cb
	op.callback = cb
	// completion
	op.completion = completeSendTo

	// cylinder
	cylinder := nextIOURingCylinder()

	// timeout
	//if timeout := op.timeout; timeout > 0 {
	//	timer := getOperatorTimer()
	//	op.timer = timer
	//	timer.Start(timeout, &operatorCanceler{
	//		cylinder: cylinder,
	//		op:       &op,
	//	})
	//}

	sqe, sqeErr := cylinder.getSQE()
	if sqeErr != nil {
		cb(0, op.userdata, os.NewSyscallError("io_uring_prep_sendto", sqeErr))
		op.callback = nil
		op.completion = nil
		return
	}

	// timeout
	if timeout := op.timeout; timeout > 0 {
		timer := getOperatorTimer()
		op.timer = timer
		timer.Start(timeout, &operatorCanceler{
			cylinder: cylinder,
			op:       &op,
		})
	}

	sqe.OpCode = opSend
	sqe.Flags = 0
	sqe.IoPrio = 0
	sqe.Fd = int32(fd.Fd())
	sqe.Off = uint64(uintptr(unsafe.Pointer(msg.Name)))
	sqe.Addr = uint64(bufAddr)
	sqe.Len = bufLen
	sqe.UserData = uint64(uintptr(unsafe.Pointer(&op)))
	sqe.BufIG = 0
	sqe.Personality = 0
	sqe.SpliceFdIn = int32(msg.Namelen)

	// prepare
	//err := cylinder.prepare(opSend, fd.Fd(), bufAddr, bufLen, 0, 0, &op)
	runtime.KeepAlive(&op)
	//if err != nil {
	//	cb(0, op.userdata, os.NewSyscallError("io_uring_prep_sendto", err))
	//	// reset
	//	op.callback = nil
	//	op.completion = nil
	//	if op.timer != nil {
	//		timer := op.timer
	//		timer.Done()
	//		putOperatorTimer(timer)
	//		op.timer = nil
	//	}
	//	return
	//}
	return
}

func completeSendTo(result int, op *Operator, err error) {
	if err != nil {
		err = os.NewSyscallError("io_uring_prep_sendto", err)
	}
	op.callback(result, op.userdata, err)
	return
}

func SendMsg(fd NetFd, b []byte, oob []byte, addr net.Addr, cb OperationCallback) {
	// op
	op := fd.WriteOperator()
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
	_ = msg.Append(b)
	msg.SetControl(oob)
	_, saErr := msg.SetAddr(addr)
	if saErr != nil {
		cb(0, op.userdata, saErr)
		return
	}

	op.userdata.Msg = &msg

	// cb
	op.callback = cb
	// completion
	op.completion = completeSendMsg

	// cylinder
	cylinder := nextIOURingCylinder()

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
	err := cylinder.prepare(opSendMsgZC, fd.Fd(), uintptr(unsafe.Pointer(&msg)), 1, 0, 0, &op)
	runtime.KeepAlive(&op)
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
