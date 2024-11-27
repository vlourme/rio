//go:build windows

package aio

import (
	"errors"
	"golang.org/x/sys/windows"
	"syscall"
	"unsafe"
)

func Recv(fd NetFd, b []byte, cb OperationCallback) {
	bLen := len(b)
	if bLen == 0 {
		cb(0, Userdata{}, ErrEmptyBytes)
		return
	} else if bLen > maxRW {
		b = b[:maxRW]
		bLen = maxRW
	}
	// op
	op := fd.ReadOperator()
	// buf
	op.userdata.Msg.AppendBuffer(b)
	wsabuf := (*syscall.WSABuf)(unsafe.Pointer(op.userdata.Msg.Buffers))
	// cb
	op.callback = cb
	// completion
	op.completion = completeRecv

	// overlapped
	overlapped := &op.overlapped
	// timeout
	if timeout := op.timeout; timeout > 0 {
		timer := getOperatorTimer()
		op.timer = timer
		timer.Start(timeout, &operatorCanceler{
			handle:     syscall.Handle(fd.Fd()),
			overlapped: overlapped,
		})
	}

	// recv
	err := syscall.WSARecv(
		syscall.Handle(fd.Fd()),
		wsabuf, op.userdata.Msg.BufferCount,
		&op.userdata.QTY, &op.userdata.Msg.Flags,
		overlapped,
		nil,
	)
	if err != nil && !errors.Is(windows.ERROR_IO_PENDING, err) {
		// handle err
		cb(0, op.userdata, errors.Join(errors.New("aio: recv failed"), err))
		// reset
		op.callback = nil
		op.completion = nil
		if op.timer != nil {
			timer := op.timer
			timer.Done()
			putOperatorTimer(timer)
			op.timer = nil
		}
	}
	return
}

func completeRecv(result int, op *Operator, err error) {
	op.callback(result, op.userdata, eofError(op.fd, result, err))
	return
}

func RecvFrom(fd NetFd, b []byte, cb OperationCallback) {
	// todo: try test recv from over recv msg
	return
}

func completeRecvFrom(result int, op *Operator, err error) {
	op.callback(result, op.userdata, err)
	return
}

func RecvMsg(fd NetFd, b []byte, oob []byte, cb OperationCallback) {

	return
}

func completeRecvMsg(result int, op *Operator, err error) {
	op.callback(result, op.userdata, err)
	return
}
