//go:build windows

package aio

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"syscall"
	"unsafe"
)

func Recv(fd NetFd, b []byte, cb OperationCallback) {
	// check buf
	bLen := len(b)
	if bLen == 0 {
		cb(-1, Userdata{}, ErrEmptyBytes)
		return
	} else if bLen > MaxRW {
		b = b[:MaxRW]
	}
	// op
	op := fd.ReadOperator()
	// msg
	buf := op.userdata.Msg.Append(b)

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
		&buf, op.userdata.Msg.BufferCount,
		&op.userdata.QTY, &op.userdata.Msg.WSAMsg.Flags,
		overlapped,
		nil,
	)
	if err != nil && !errors.Is(syscall.ERROR_IO_PENDING, err) {
		// handle err
		cb(-1, Userdata{}, os.NewSyscallError("wsa_recv", err))
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
	if err != nil {
		err = os.NewSyscallError("wsa_recv", err)
	}
	op.callback(result, op.userdata, eofError(op.fd, result, err))
	return
}

func RecvFrom(fd NetFd, b []byte, cb OperationCallback) {
	// check buf
	bLen := len(b)
	if bLen == 0 {
		cb(-1, Userdata{}, ErrEmptyBytes)
		return
	}
	// op
	op := fd.ReadOperator()
	// msg
	addr, addrLen := op.userdata.Msg.BuildRawSockaddrAny()
	buf := op.userdata.Msg.Append(b)
	// cb
	op.callback = cb
	// completion
	op.completion = completeRecvFrom

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

	// recv from
	err := syscall.WSARecvFrom(
		syscall.Handle(fd.Fd()),
		&buf, op.userdata.Msg.BufferCount,
		&op.userdata.QTY, &op.userdata.Msg.WSAMsg.Flags,
		addr, &addrLen,
		overlapped,
		nil,
	)
	if err != nil && !errors.Is(syscall.ERROR_IO_PENDING, err) {
		// handle err
		cb(-1, Userdata{}, os.NewSyscallError("wsa_recvfrom", err))
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

func completeRecvFrom(result int, op *Operator, err error) {
	if err != nil {
		err = os.NewSyscallError("wsa_recvfrom", err)
	}
	op.callback(result, op.userdata, err)
	return
}

func RecvMsg(fd NetFd, b []byte, oob []byte, cb OperationCallback) {
	// check buf
	bLen := len(b)
	if bLen == 0 {
		cb(-1, Userdata{}, ErrEmptyBytes)
		return
	}
	// op
	op := fd.ReadOperator()
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
	op.completion = completeRecvMsg

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

	// recv msg
	err := windows.WSARecvMsg(
		windows.Handle(fd.Fd()),
		&op.userdata.Msg.WSAMsg,
		&op.userdata.QTY,
		wsaoverlapped,
		nil,
	)
	if err != nil && !errors.Is(windows.ERROR_IO_PENDING, err) {
		// handle err
		cb(-1, Userdata{}, os.NewSyscallError("wsa_recvmsg", err))
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

func completeRecvMsg(result int, op *Operator, err error) {
	if err != nil {
		err = os.NewSyscallError("wsa_recvmsg", err)
	}
	op.callback(result, op.userdata, err)
	return
}
