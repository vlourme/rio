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
	// op
	op := fd.prepareWriting()
	if op == nil {
		cb(Userdata{}, errors.New("operator padding"))
		return
	}
	// msg
	bLen := len(b)
	if bLen > maxRW {
		b = b[:maxRW]
	}
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
		fd.finishWriting()
		return
	}
	return
}

func completeSend(result int, op *Operator, err error) {
	cb := op.callback
	fd := op.fd
	fd.finishWriting()
	if err != nil {
		err = os.NewSyscallError("wsa_send", err)
		cb(Userdata{}, err)
		return
	}
	cb(Userdata{N: result}, nil)
	return
}

func SendTo(fd NetFd, b []byte, addr net.Addr, cb OperationCallback) {
	// op
	op := fd.prepareWriting()
	if op == nil {
		cb(Userdata{}, errors.New("operator padding"))
		return
	}
	// msg
	bLen := len(b)
	if bLen > maxRW {
		b = b[:maxRW]
	}
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
		fd.finishWriting()
		return
	}
	return
}

func completeSendTo(result int, op *Operator, err error) {
	cb := op.callback
	fd := op.fd
	fd.finishWriting()
	if err != nil {
		err = os.NewSyscallError("wsa_sendto", err)
		cb(Userdata{}, err)
		return
	}
	cb(Userdata{N: result}, nil)
	return
}

func SendMsg(fd NetFd, b []byte, oob []byte, addr net.Addr, cb OperationCallback) {
	sa := AddrToSockaddr(addr)
	rsa, rsaLen, rsaErr := SockaddrToRaw(sa)
	if rsaErr != nil {
		cb(Userdata{}, errors.Join(errors.New("aio: send msg failed"), rsaErr))
		return
	}

	// op
	op := fd.prepareWriting()
	if op == nil {
		cb(Userdata{}, errors.New("operator padding"))
		return
	}
	// msg
	bLen := len(b)
	if bLen > maxRW {
		cb(Userdata{}, errors.New("packet is too large (only 1GB is allowed)"))
		return
	}
	op.msg.Name = rsa
	op.msg.Namelen = rsaLen

	op.msg.Buffers = &windows.WSABuf{
		Len: uint32(bLen),
		Buf: nil,
	}
	op.msg.BufferCount = 1

	if bLen > 0 {
		op.msg.Buffers.Buf = &b[0]
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
		&op.msg,
		op.msg.Flags,
		&op.n,
		wsaoverlapped,
		nil,
	)
	if err != nil && !errors.Is(windows.ERROR_IO_PENDING, err) {
		// handle err
		cb(Userdata{}, os.NewSyscallError("wsa_sendmsg", err))
		// fin op
		fd.finishWriting()
		return
	}
	return
}

func completeSendMsg(result int, op *Operator, err error) {
	cb := op.callback
	msg := op.msg
	fd := op.fd
	fd.finishWriting()
	if err != nil {
		err = os.NewSyscallError("wsa_sendmsg", err)
		cb(Userdata{}, err)
		return
	}
	oobn := int(msg.Control.Len)
	cb(Userdata{N: result, OOBN: oobn}, nil)
	return
}
