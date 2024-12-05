//go:build windows

package aio

import (
	"errors"
	"golang.org/x/sys/windows"
	"net"
	"syscall"
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
	msg := WSAMessage{}
	buf := msg.Append(b)
	op.userdata.Msg = &msg

	// cb
	op.callback = cb
	// completion
	op.completion = completeSend

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

	// send
	err := syscall.WSASend(
		syscall.Handle(fd.Fd()),
		&buf, msg.BufferCount,
		&op.userdata.QTY, msg.WSAMsg.Flags,
		overlapped,
		nil,
	)
	if err != nil && !errors.Is(syscall.ERROR_IO_PENDING, err) {
		// handle err
		cb(0, op.userdata, errors.Join(errors.New("aio: send failed"), err))
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

func completeSend(result int, op *Operator, err error) {
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
	msg := WSAMessage{}
	sa, saErr := msg.SetAddr(addr)
	if saErr != nil {
		cb(0, op.userdata, errors.Join(errors.New("aio: send to failed"), saErr))
		return
	}
	buf := msg.Append(b)
	op.userdata.Msg = &msg

	// cb
	op.callback = cb
	// completion
	op.completion = completeSendTo

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

	// send to
	err := syscall.WSASendto(
		syscall.Handle(fd.Fd()),
		&buf, msg.BufferCount,
		&op.userdata.QTY, msg.WSAMsg.Flags,
		sa,
		overlapped,
		nil,
	)
	if err != nil && !errors.Is(syscall.ERROR_IO_PENDING, err) {
		// handle err
		cb(0, op.userdata, errors.Join(errors.New("aio: send to failed"), err))
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

func completeSendTo(result int, op *Operator, err error) {
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
	msg := WSAMessage{}
	msg.Append(b)
	msg.SetControl(oob)
	_, saErr := msg.SetAddr(addr)
	if saErr != nil {
		cb(0, op.userdata, errors.Join(errors.New("aio: send msg failed"), saErr))
		return
	}
	op.userdata.Msg = &msg

	// cb
	op.callback = cb
	// completion
	op.completion = completeSendMsg

	// overlapped
	overlapped := &op.overlapped
	wsaoverlapped := (*windows.Overlapped)(unsafe.Pointer(overlapped))
	// timeout
	if timeout := op.timeout; timeout > 0 {
		timer := getOperatorTimer()
		op.timer = timer
		timer.Start(timeout, &operatorCanceler{
			handle:     syscall.Handle(fd.Fd()),
			overlapped: overlapped,
		})
	}

	// send msg
	err := windows.WSASendMsg(
		windows.Handle(fd.Fd()),
		&msg.WSAMsg,
		msg.WSAMsg.Flags,
		&op.userdata.QTY,
		wsaoverlapped,
		nil,
	)
	if err != nil && !errors.Is(windows.ERROR_IO_PENDING, err) {
		// handle err
		cb(0, op.userdata, errors.Join(errors.New("aio: send msg failed"), err))
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

func completeSendMsg(result int, op *Operator, err error) {
	op.callback(result, op.userdata, err)
	return
}
