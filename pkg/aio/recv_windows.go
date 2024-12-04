//go:build windows

package aio

import (
	"errors"
	"golang.org/x/sys/windows"
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
	buf := op.userdata.Msg.AppendBuffer(b)
	wsabuf := (*syscall.WSABuf)(unsafe.Pointer(&buf))
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
	if err != nil && !errors.Is(syscall.ERROR_IO_PENDING, err) {
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

	wsamsg := windows.WSAMsg{}
	// addr
	wsamsg.Name = new(syscall.RawSockaddrAny)
	wsamsg.Namelen = int32(unsafe.Sizeof(*wsamsg.Name))
	waddr := (*windows.RawSockaddrAny)(unsafe.Pointer(wsamsg.Name))
	// buf
	wsamsg.Buffers = &windows.WSABuf{
		Len: uint32(len(b)),
		Buf: &b[0],
	}
	wsamsg.BufferCount = 1

	// flags
	wsamsg.Flags = 0

	op.userdata.msg = uintptr(unsafe.Pointer(&wsamsg))
	// cb
	op.callback = cb
	// completion
	op.completion = completeRecvFrom

	// overlapped
	overlapped := &op.overlapped
	wsaOverlapped := (*windows.Overlapped)(unsafe.Pointer(overlapped))
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
	err := windows.WSARecvFrom(
		windows.Handle(fd.Fd()),
		wsamsg.Buffers, wsamsg.BufferCount,
		&op.userdata.QTY, &wsamsg.Flags,
		waddr, &wsamsg.Namelen,
		wsaOverlapped,
		nil,
	)
	if err != nil && !errors.Is(syscall.ERROR_IO_PENDING, err) {
		// handle err
		cb(0, op.userdata, errors.Join(errors.New("aio: recv from failed"), err))
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
	op.callback(result, op.userdata, err)
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
	wsamsg := windows.WSAMsg{}
	// addr
	wsamsg.Name = new(syscall.RawSockaddrAny)
	wsamsg.Namelen = int32(unsafe.Sizeof(*wsamsg.Name))
	// buf
	wsamsg.Buffers = &windows.WSABuf{
		Len: uint32(len(b)),
		Buf: &b[0],
	}
	wsamsg.BufferCount = 1
	// oob
	wsamsg.Control.Len = uint32(len(oob))
	if wsamsg.Control.Len > 0 {
		wsamsg.Control.Buf = &oob[0]
	}
	// flags
	wsamsg.Flags = 0
	if fd.Family() == syscall.AF_UNIX {
		wsamsg.Flags = wsamsg.Flags | readMsgFlags
	}

	op.userdata.msg = uintptr(unsafe.Pointer(&wsamsg))
	// cb
	op.callback = cb
	// completion
	op.completion = completeRecvMsg

	// overlapped
	overlapped := &op.overlapped
	wsaOverlapped := (*windows.Overlapped)(unsafe.Pointer(overlapped))
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
		&wsamsg,
		&op.userdata.QTY,
		wsaOverlapped,
		nil,
	)
	if err != nil && !errors.Is(windows.ERROR_IO_PENDING, err) {
		// handle err
		cb(0, op.userdata, errors.Join(errors.New("aio: recv msg failed"), err))
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
	op.callback(result, op.userdata, err)
	return
}
