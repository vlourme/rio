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
	op.begin()
	// msg
	buf := syscall.WSABuf{
		Len: uint32(bLen),
		Buf: &b[0],
	}

	// cb
	op.callback = cb
	// completion
	op.completion = completeSend

	// overlapped
	overlapped := &op.overlapped

	// send
	err := syscall.WSASend(
		syscall.Handle(fd.Fd()),
		&buf, 1,
		&op.n, 0,
		overlapped,
		nil,
	)
	if err != nil && !errors.Is(syscall.ERROR_IO_PENDING, err) {
		// handle err
		cb(Userdata{}, os.NewSyscallError("wsa_send", err))
		// reset op
		op.reset()
	}
	return
}

func completeSend(result int, op *Operator, err error) {
	if err != nil {
		err = os.NewSyscallError("wsa_send", err)
		op.callback(Userdata{}, err)
		return
	}
	op.callback(Userdata{N: result}, nil)
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
	op.begin()
	// msg
	buf := syscall.WSABuf{
		Len: uint32(bLen),
		Buf: &b[0],
	}
	sa := AddrToSockaddr(addr)

	// cb
	op.callback = cb
	// completion
	op.completion = completeSendTo

	// overlapped
	overlapped := &op.overlapped

	// send to
	err := syscall.WSASendto(
		syscall.Handle(fd.Fd()),
		&buf, 1,
		&op.n, 0,
		sa,
		overlapped,
		nil,
	)
	if err != nil && !errors.Is(syscall.ERROR_IO_PENDING, err) {
		// handle err
		cb(Userdata{}, os.NewSyscallError("wsa_sendto", err))
		// reset op
		op.reset()
	}
	return
}

func completeSendTo(result int, op *Operator, err error) {
	if err != nil {
		err = os.NewSyscallError("wsa_sendto", err)
		op.callback(Userdata{}, err)
		return
	}
	op.callback(Userdata{N: result}, nil)
	return
}

func SendMsg(fd NetFd, b []byte, oob []byte, addr net.Addr, cb OperationCallback) {
	// check buf
	bLen := len(b)
	if bLen == 0 {
		cb(Userdata{}, ErrEmptyBytes)
		return
	}
	sa := AddrToSockaddr(addr)
	rsa, rsaLen, rsaErr := SockaddrToRaw(sa)
	if rsaErr != nil {
		cb(Userdata{}, errors.Join(errors.New("aio: send msg failed"), rsaErr))
		return
	}

	// op
	op := fd.WriteOperator()
	op.begin()
	// msg
	op.msg = &windows.WSAMsg{
		Name:    rsa,
		Namelen: rsaLen,
		Buffers: &windows.WSABuf{
			Len: uint32(bLen),
			Buf: &b[0],
		},
		BufferCount: 1,
		Control:     windows.WSABuf{},
		Flags:       0,
	}
	if oobLen := len(oob); oobLen > 0 {
		op.msg.Control.Len = uint32(oobLen)
		op.msg.Control.Buf = &oob[0]
	}

	// cb
	op.callback = cb
	// completion
	op.completion = completeSendMsg

	// overlapped
	overlapped := &op.overlapped
	wsaoverlapped := (*windows.Overlapped)(unsafe.Pointer(overlapped))

	// send msg
	err := windows.WSASendMsg(
		windows.Handle(fd.Fd()),
		op.msg,
		op.msg.Flags,
		&op.n,
		wsaoverlapped,
		nil,
	)
	if err != nil && !errors.Is(windows.ERROR_IO_PENDING, err) {
		// handle err
		cb(Userdata{}, os.NewSyscallError("wsa_sendmsg", err))
		// reset op
		op.reset()
	}
	return
}

func completeSendMsg(result int, op *Operator, err error) {
	if err != nil {
		err = os.NewSyscallError("wsa_sendmsg", err)
		op.callback(Userdata{}, err)
		return
	}
	oobn := int(op.msg.Control.Len)
	op.callback(Userdata{N: result, OOBN: oobn}, nil)
	return
}
