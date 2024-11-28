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
	// buf
	bLen := len(b)
	if bLen == 0 {
		cb(0, op.userdata, ErrEmptyBytes)
		return
	} else if bLen > maxRW {
		b = b[:maxRW]
		bLen = maxRW
	}
	op.userdata.Msg.AppendBuffer(b)
	wsabuf := (*syscall.WSABuf)(unsafe.Pointer(op.userdata.Msg.Buffers))
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
		wsabuf, op.userdata.Msg.BufferCount,
		&op.userdata.QTY, op.userdata.Msg.Flags,
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
	// buf
	bLen := len(b)
	if bLen == 0 {
		cb(0, op.userdata, ErrEmptyBytes)
		return
	} else if bLen > maxRW {
		b = b[:maxRW]
		bLen = maxRW
	}
	op.userdata.Msg.AppendBuffer(b)
	wsabuf := (*syscall.WSABuf)(unsafe.Pointer(op.userdata.Msg.Buffers))
	// addr
	sa := AddrToSockaddr(addr)
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
		wsabuf, op.userdata.Msg.BufferCount,
		&op.userdata.QTY, op.userdata.Msg.Flags,
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
	// buf
	bLen := len(b)
	if bLen == 0 {
		cb(0, op.userdata, ErrEmptyBytes)
		return
	} else if bLen > maxRW {
		b = b[:maxRW]
		bLen = maxRW
	}
	op.userdata.Msg.AppendBuffer(b)
	op.userdata.Msg.SetControl(oob)
	// addr
	if addr != nil {
		sa := AddrToSockaddr(addr)
		rsa, rsaLen, rsaErr := SockaddrToRaw(sa)
		if rsaErr != nil {
			cb(0, op.userdata, rsaErr)
			return
		}
		op.userdata.Msg.Name = rsa
		op.userdata.Msg.Namelen = rsaLen
	}
	wsamsg := (*windows.WSAMsg)(unsafe.Pointer(&op.userdata.Msg))
	// cb
	op.callback = cb
	// completion
	op.completion = completeSendMsg

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

	// send msg
	err := windows.WSASendMsg(
		windows.Handle(fd.Fd()),
		wsamsg,
		op.userdata.Msg.Flags,
		&op.userdata.QTY,
		wsaOverlapped,
		nil,
	)
	if err != nil && !errors.Is(syscall.ERROR_IO_PENDING, err) {
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
