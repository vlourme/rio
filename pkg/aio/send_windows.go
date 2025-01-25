//go:build windows

package aio

import (
	"errors"
	"golang.org/x/sys/windows"
	"net"
	"os"
	"syscall"
	"unsafe"
)

func Send(fd NetFd, b []byte, cb OperationCallback) {
	// check buf
	bLen := len(b)
	if bLen == 0 {
		cb(Userdata{}, ErrEmptyBytes)
		return
	} else if bLen > MaxRW {
		b = b[:MaxRW]
	}
	// op
	op := fd.WriteOperator()
	// msg
	buf := op.msg.Append(b)

	// cb
	op.callback = cb
	// completion
	op.completion = completeSend

	// timeout
	op.tryPrepareTimeout()

	// overlapped
	overlapped := &op.overlapped

	// send
	err := syscall.WSASend(
		syscall.Handle(fd.Fd()),
		&buf, op.msg.BufferCount,
		&op.n, op.msg.WSAMsg.Flags,
		overlapped,
		nil,
	)
	if err != nil && !errors.Is(syscall.ERROR_IO_PENDING, err) {
		// handle err
		cb(Userdata{}, os.NewSyscallError("wsa_send", err))
		// clean op
		op.clean()
	}
	return
}

func completeSend(result int, op *Operator, err error) {
	if err != nil {
		err = os.NewSyscallError("wsa_send", err)
		op.callback(Userdata{}, err)
		return
	}
	op.callback(Userdata{QTY: result}, nil)
	return
}

func SendTo(fd NetFd, b []byte, addr net.Addr, cb OperationCallback) {
	// check buf
	bLen := len(b)
	if bLen == 0 {
		cb(Userdata{}, ErrEmptyBytes)
		return
	}
	// op
	op := fd.WriteOperator()
	// msg
	sa, saErr := op.msg.SetAddr(addr)
	if saErr != nil {
		cb(Userdata{}, errors.Join(errors.New("aio: send to failed"), saErr))
		op.clean()
		return
	}
	buf := op.msg.Append(b)

	// cb
	op.callback = cb
	// completion
	op.completion = completeSendTo

	// timeout
	op.tryPrepareTimeout()

	// overlapped
	overlapped := &op.overlapped

	// send to
	err := syscall.WSASendto(
		syscall.Handle(fd.Fd()),
		&buf, op.msg.BufferCount,
		&op.n, op.msg.WSAMsg.Flags,
		sa,
		overlapped,
		nil,
	)
	if err != nil && !errors.Is(syscall.ERROR_IO_PENDING, err) {
		// handle err
		cb(Userdata{}, os.NewSyscallError("wsa_sendto", err))
		// clean op
		op.clean()
	}
	return
}

func completeSendTo(result int, op *Operator, err error) {
	if err != nil {
		err = os.NewSyscallError("wsa_sendto", err)
		op.callback(Userdata{}, err)
		return
	}
	op.callback(Userdata{QTY: result, Msg: op.msg}, nil)
	return
}

func SendMsg(fd NetFd, b []byte, oob []byte, addr net.Addr, cb OperationCallback) {
	// check buf
	bLen := len(b)
	if bLen == 0 {
		cb(Userdata{}, ErrEmptyBytes)
		return
	}
	// op
	op := fd.WriteOperator()
	// msg
	op.msg.Append(b)
	op.msg.SetControl(oob)
	_, saErr := op.msg.SetAddr(addr)
	if saErr != nil {
		cb(Userdata{}, errors.Join(errors.New("aio: send msg failed"), saErr))
		op.clean()
		return
	}

	// cb
	op.callback = cb
	// completion
	op.completion = completeSendMsg

	// timeout
	op.tryPrepareTimeout()

	// overlapped
	overlapped := &op.overlapped
	wsaoverlapped := (*windows.Overlapped)(unsafe.Pointer(overlapped))

	// send msg
	err := windows.WSASendMsg(
		windows.Handle(fd.Fd()),
		&op.msg.WSAMsg,
		op.msg.WSAMsg.Flags,
		&op.n,
		wsaoverlapped,
		nil,
	)
	if err != nil && !errors.Is(windows.ERROR_IO_PENDING, err) {
		// handle err
		cb(Userdata{}, os.NewSyscallError("wsa_sendmsg", err))
		// clean op
		op.clean()
	}
	return
}

func completeSendMsg(result int, op *Operator, err error) {
	if err != nil {
		err = os.NewSyscallError("wsa_sendmsg", err)
		op.callback(Userdata{}, err)
		return
	}
	op.callback(Userdata{QTY: result, Msg: op.msg}, nil)
	return
}
