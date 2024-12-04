//go:build linux

package aio

import (
	"syscall"
	"unsafe"
)

func Recv(fd NetFd, b []byte, cb OperationCallback) {
	// op
	op := fd.ReadOperator()
	// buf
	bLen := len(b)
	if bLen == 0 {
		cb(0, op.userdata, ErrEmptyBytes)
		return
	} else if bLen > MaxRW {
		b = b[:MaxRW]
		bLen = MaxRW
	}
	msg := Msg{}
	buf := msg.AppendBuffer(b)
	bufAddr := uintptr(unsafe.Pointer(buf.Buf))
	bufLen := uint32(buf.Len)

	op.userdata.msg = uintptr(unsafe.Pointer(&msg))
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
	// buf
	bLen := len(b)
	if bLen == 0 {
		cb(0, op.userdata, ErrEmptyBytes)
		return
	} else if bLen > MaxRW {
		b = b[:MaxRW]
		bLen = MaxRW
	}

	msg := syscall.Msghdr{}
	// addr
	msg.Name = (*byte)(unsafe.Pointer(new(syscall.RawSockaddrAny)))
	msg.Namelen = syscall.SizeofSockaddrAny
	// buf
	msg.Iov = &syscall.Iovec{
		Base: &b[0],
		Len:  uint64(len(b)),
	}
	msg.Iovlen = 1
	// oob
	if oobn := uint64(len(oob)); oobn > 0 {
		msg.Control = &oob[0]
		msg.Controllen = oobn
	}

	// flags
	msg.Flags = 0
	// handle unix
	if fd.Family() == syscall.AF_UNIX {
		msg.Flags = msg.Flags | readMsgFlags
	}

	msgPtr := uintptr(unsafe.Pointer(&msg))
	op.userdata.msg = msgPtr

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
	err := cylinder.prepare(opRecvmsg, fd.Fd(), uintptr(unsafe.Pointer(&msg)), 1, 0, 0, userdata)
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
	op.userdata.QTY = uint32(result)
	op.callback(result, op.userdata, err)
	return
}
