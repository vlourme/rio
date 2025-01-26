//go:build windows

package aio

import (
	"errors"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"syscall"
	"unsafe"
)

func Recv(fd NetFd, b []byte, cb OperationCallback) {
	// check buf
	bLen := len(b)
	if bLen == 0 {
		cb(Userdata{}, ErrEmptyBytes)
		return
	} else if bLen > MaxRW {
		b = b[:MaxRW]
	}
	// op
	op := fd.ReadOperator()
	// msg
	buf := op.msg.Append(b)

	// cb
	op.callback = cb
	// completion
	op.completion = completeRecv

	// timeout
	op.tryPrepareTimeout()

	// overlapped
	overlapped := &op.overlapped

	// recv
	err := syscall.WSARecv(
		syscall.Handle(fd.Fd()),
		&buf, op.msg.BufferCount,
		&op.n, &op.msg.WSAMsg.Flags,
		overlapped,
		nil,
	)
	if err != nil && !errors.Is(syscall.ERROR_IO_PENDING, err) {
		// handle err
		cb(Userdata{}, os.NewSyscallError("wsa_recv", err))
		// reset op
		op.reset()
	}
	return
}

func completeRecv(result int, op *Operator, err error) {
	if err != nil {
		err = os.NewSyscallError("wsa_recv", err)
		op.callback(Userdata{}, err)
		return
	}
	if result == 0 && op.fd.ZeroReadIsEOF() {
		op.callback(Userdata{}, io.EOF)
		return
	}
	op.callback(Userdata{QTY: result, Msg: op.msg}, nil)
	return
}

func RecvFrom(fd NetFd, b []byte, cb OperationCallback) {
	// check buf
	bLen := len(b)
	if bLen == 0 {
		cb(Userdata{}, ErrEmptyBytes)
		return
	}
	// op
	op := fd.ReadOperator()
	// msg
	addr, addrLen := op.msg.BuildRawSockaddrAny()
	buf := op.msg.Append(b)
	// cb
	op.callback = cb
	// completion
	op.completion = completeRecvFrom

	// timeout
	op.tryPrepareTimeout()

	// overlapped
	overlapped := &op.overlapped

	// recv from
	err := syscall.WSARecvFrom(
		syscall.Handle(fd.Fd()),
		&buf, op.msg.BufferCount,
		&op.n, &op.msg.WSAMsg.Flags,
		addr, &addrLen,
		overlapped,
		nil,
	)
	if err != nil && !errors.Is(syscall.ERROR_IO_PENDING, err) {
		// handle err
		cb(Userdata{}, os.NewSyscallError("wsa_recvfrom", err))
		// reset op
		op.reset()
	}
	return
}

func completeRecvFrom(result int, op *Operator, err error) {
	if err != nil {
		err = os.NewSyscallError("wsa_recvfrom", err)
		op.callback(Userdata{}, err)
		return
	}
	op.callback(Userdata{QTY: result, Msg: op.msg}, nil)
	return
}

func RecvMsg(fd NetFd, b []byte, oob []byte, cb OperationCallback) {
	// check buf
	bLen := len(b)
	if bLen == 0 {
		cb(Userdata{}, ErrEmptyBytes)
		return
	}
	// op
	op := fd.ReadOperator()
	// msg
	op.msg.BuildRawSockaddrAny()
	op.msg.Append(b)
	op.msg.SetControl(oob)
	if fd.Family() == syscall.AF_UNIX {
		op.msg.SetFlags(readMsgFlags)
	}

	// cb
	op.callback = cb
	// completion
	op.completion = completeRecvMsg

	// timeout
	op.tryPrepareTimeout()

	// overlapped
	overlapped := &op.overlapped
	wsaoverlapped := (*windows.Overlapped)(unsafe.Pointer(overlapped))

	// recv msg
	err := windows.WSARecvMsg(
		windows.Handle(fd.Fd()),
		&op.msg.WSAMsg,
		&op.n,
		wsaoverlapped,
		nil,
	)
	if err != nil && !errors.Is(windows.ERROR_IO_PENDING, err) {
		// handle err
		cb(Userdata{}, os.NewSyscallError("wsa_recvmsg", err))
		// reset op
		op.reset()
	}
	return
}

func completeRecvMsg(result int, op *Operator, err error) {
	if err != nil {
		err = os.NewSyscallError("wsa_recvmsg", err)
		op.callback(Userdata{}, err)
		return
	}
	op.callback(Userdata{QTY: result, Msg: op.msg}, nil)
	return
}
