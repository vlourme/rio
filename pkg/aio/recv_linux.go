//go:build linux

package aio

import "unsafe"

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
	buf := op.userdata.Msg.AppendBuffer(b)
	bufAddr := uintptr(unsafe.Pointer(buf.Buf))
	bufLen := buf.Len
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

	return
}

func completeRecvFrom(result int, op *Operator, err error) {
	op.userdata.QTY = uint32(result)
	op.callback(result, op.userdata, err)
	return
}

func RecvMsg(fd NetFd, b []byte, oob []byte, cb OperationCallback) {

	return
}

func completeRecvMsg(result int, op *Operator, err error) {
	op.userdata.QTY = uint32(result)
	op.callback(result, op.userdata, err)
	return
}
