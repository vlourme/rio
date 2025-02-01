//go:build linux

package aio

import (
	"errors"
	"net"
	"os"
	"syscall"
	"unsafe"
)

// Send
// send_zc: available since 6.0
// sendmsg_zc: available since 6.1
// https://manpages.debian.org/unstable/liburing-dev/io_uring_enter.2.en.html
func Send(fd NetFd, b []byte, cb OperationCallback) {
	op := fd.WriteOperator()

	// msg
	bLen := len(b)
	if bLen > MaxRW {
		b = b[:MaxRW]
	}
	bufAddr := uintptr(unsafe.Pointer(&b[0]))
	bufLen := uint32(bLen)

	// cb
	op.callback = cb
	// completion
	op.completion = completeSend

	// cylinder
	cylinder := nextIOURingCylinder()
	op.setCylinder(cylinder)

	// prepare
	err := cylinder.prepareRW(opSend, fd.Fd(), bufAddr, bufLen, 0, 0, op.ptr())
	if err != nil {
		cb(Userdata{}, os.NewSyscallError("io_uring_prep_send", err))
		op.reset()
		return
	}
	return
}

func completeSend(result int, op *Operator, err error) {
	if err != nil {
		err = os.NewSyscallError("io_uring_prep_send", err)
		op.callback(Userdata{}, err)
		return
	}
	op.callback(Userdata{N: result}, nil)
	return
}

func SendTo(fd NetFd, b []byte, addr net.Addr, cb OperationCallback) {
	SendMsg(fd, b, nil, addr, cb)
}

func SendMsg(fd NetFd, b []byte, oob []byte, addr net.Addr, cb OperationCallback) {
	sa := AddrToSockaddr(addr)
	rsa, rsaLen, rsaErr := SockaddrToRaw(sa)
	if rsaErr != nil {
		cb(Userdata{}, errors.Join(errors.New("aio: send msg failed"), rsaErr))
		return
	}
	// op
	op := fd.WriteOperator()
	// msg
	bLen := len(b)
	if bLen > MaxRW {
		b = b[:MaxRW]
	}
	op.msg = &syscall.Msghdr{
		Name:      (*byte)(unsafe.Pointer(rsa)),
		Namelen:   uint32(rsaLen),
		Pad_cgo_0: [4]byte{},
		Iov: &syscall.Iovec{
			Base: &b[0],
			Len:  uint64(bLen),
		},
		Iovlen:     1,
		Control:    nil,
		Controllen: 0,
		Flags:      0,
		Pad_cgo_1:  [4]byte{},
	}
	if oobLen := len(oob); oobLen > 0 {
		if oobLen > 64 {
			oob = oob[:64]
			oobLen = 64
		}
		op.msg.Control = &oob[0]
		op.msg.Controllen = uint64(oobLen)
	}

	// cb
	op.callback = cb
	// completion
	op.completion = completeSendMsg
	// cylinder
	cylinder := nextIOURingCylinder()
	op.setCylinder(cylinder)

	// prepare
	err := cylinder.prepareRW(opSendmsg, fd.Fd(), uintptr(unsafe.Pointer(op.msg)), 1, 0, 0, op.ptr())
	if err != nil {
		cb(Userdata{}, os.NewSyscallError("io_uring_prep_sendmsg", err))
		op.reset()
		return
	}
	return
}

func completeSendMsg(result int, op *Operator, err error) {
	if err != nil {
		err = os.NewSyscallError("io_uring_prep_sendmsg", err)
		op.callback(Userdata{}, err)
		return
	}
	oobn := int(op.msg.Controllen)
	op.callback(Userdata{N: result, OOBN: oobn}, nil)
	return
}
