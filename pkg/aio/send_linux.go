//go:build linux

package aio

import (
	"errors"
	"net"
	"os"
	"syscall"
	"unsafe"
)

var opSendCode uint8 = 0

// Send
// send_zc: available since 6.0
// sendmsg_zc: available since 6.1
// https://manpages.debian.org/unstable/liburing-dev/io_uring_enter.2.en.html
func Send(fd NetFd, b []byte, cb OperationCallback) {
	bLen := len(b)
	if bLen == 0 {
		cb(Userdata{}, nil)
		return
	}
	// op
	op := acquireOperator(fd)
	// cb
	op.callback = cb
	// buf
	bufAddr := uintptr(unsafe.Pointer(&b[0]))
	bufLen := uint32(bLen)
	// sock
	sock := fd.Fd()

	// op code
	switch opSendCode {
	case opSend:
		// completion
		op.completion = completeSend
		// cylinder
		cylinder := nextIOURingCylinder()
		op.setCylinder(cylinder)
		err := cylinder.prepareRW(opSend, sock, bufAddr, bufLen, 0, 0, op.ptr())
		if err != nil {
			cb(Userdata{}, os.NewSyscallError("io_uring_prep_send", err))
			releaseOperator(op)
		}
		break
	case opSendZC:
		op.b = b
		// completion
		op.completion = completeSendZC
		// cylinder
		cylinder := nextIOURingCylinder()
		op.setCylinder(cylinder)
		err := cylinder.prepareRW(opSendZC, sock, bufAddr, bufLen, 0, 0, op.ptr())
		if err != nil {
			cb(Userdata{}, os.NewSyscallError("io_uring_prep_send_zc", err))
			releaseOperator(op)
		}
		break
	default:
		cb(Userdata{}, errors.New("invalid opSendCode"))
		releaseOperator(op)
		break
	}
	return
}

func completeSend(result int, op *Operator, err error) {
	cb := op.callback
	releaseOperator(op)
	if err != nil {
		err = os.NewSyscallError("io_uring_prep_send", err)
		cb(Userdata{}, err)
		return
	}
	cb(Userdata{N: result}, nil)
	return
}

func completeSendZC(result int, op *Operator, err error) {
	cb := op.callback
	cqeFlags := op.cqeFlags
	if err != nil {
		releaseOperator(op)
		if cb != nil {
			err = os.NewSyscallError("io_uring_prep_send_zc", err)
			cb(Userdata{}, err)
		}
		return
	}
	if cqeFlags&cqeFMore != 0 {
		op.callback = nil
		cb(Userdata{N: result}, nil)
		return
	}
	if op.cqeFlags&cqeFNotification != 0 {
		releaseOperator(op)
		return
	}
	releaseOperator(op)
	if cb != nil {
		err = os.NewSyscallError("io_uring_prep_send_zc", errors.New("invalid cqe flags"))
		cb(Userdata{}, err)
	}
	return
}

func SendTo(fd NetFd, b []byte, addr net.Addr, cb OperationCallback) {
	SendMsg(fd, b, nil, addr, cb)
}

var opSendMsgCode uint8 = 0

func SendMsg(fd NetFd, b []byte, oob []byte, addr net.Addr, cb OperationCallback) {
	sa := AddrToSockaddr(addr)
	rsa, rsaLen, rsaErr := SockaddrToRaw(sa)
	if rsaErr != nil {
		cb(Userdata{}, errors.Join(errors.New("aio: send msg failed"), rsaErr))
		return
	}
	// op
	op := acquireOperator(fd)
	// cb
	op.callback = cb
	// msg
	op.msg.Name = (*byte)(unsafe.Pointer(rsa))
	op.msg.Namelen = uint32(rsaLen)
	bLen := len(b)
	if bLen > 0 {
		op.msg.Iov = &syscall.Iovec{
			Base: &b[0],
			Len:  uint64(bLen),
		}
		op.msg.Iovlen = 1
	}
	if oobLen := len(oob); oobLen > 0 {
		op.msg.Control = &oob[0]
		op.msg.Controllen = uint64(oobLen)
		if bLen == 0 && fd.SocketType() != syscall.SOCK_DGRAM {
			var dummy byte
			op.msg.Iov.Base = &dummy
			op.msg.Iov.Len = uint64(1)
			op.msg.Iovlen = 1
		}
	}
	// op code
	switch opSendMsgCode {
	case opSendmsg:
		// completion
		op.completion = completeSendMsg
		// cylinder
		cylinder := nextIOURingCylinder()
		op.setCylinder(cylinder)
		err := cylinder.prepareRW(opSendmsg, fd.Fd(), uintptr(unsafe.Pointer(&op.msg)), 1, 0, 0, op.ptr())
		if err != nil {
			cb(Userdata{}, os.NewSyscallError("io_uring_prep_sendmsg", err))
			releaseOperator(op)
		}
		break
	case opSendMsgZC:
		// b
		op.b = b
		// completion
		op.completion = completeSendMsgZC
		// cylinder
		cylinder := nextIOURingCylinder()
		op.setCylinder(cylinder)
		err := cylinder.prepareRW(opSendMsgZC, fd.Fd(), uintptr(unsafe.Pointer(&op.msg)), 1, 0, 0, op.ptr())
		if err != nil {
			cb(Userdata{}, os.NewSyscallError("io_uring_prep_sendmsg_zc", err))
			releaseOperator(op)
		}
		break
	default:
		releaseOperator(op)
		cb(Userdata{}, errors.New("invalid opSendMsgCode"))
		break
	}
	return
}

func completeSendMsg(result int, op *Operator, err error) {
	cb := op.callback
	msg := op.msg
	releaseOperator(op)
	if err != nil {
		err = os.NewSyscallError("io_uring_prep_sendmsg", err)
		cb(Userdata{}, err)
		return
	}
	bLen := 0
	if msg.Iov != nil {
		bLen = int(msg.Iov.Len)
	}
	oobn := int(msg.Controllen)
	if oobn > 0 && bLen == 0 {
		result = 0
	}
	cb(Userdata{N: result, OOBN: oobn}, nil)
	return
}

func completeSendMsgZC(result int, op *Operator, err error) {
	defer releaseOperator(op)
	cb := op.callback
	cqeFlags := op.cqeFlags
	msg := op.msg

	if err != nil {
		releaseOperator(op)
		if cb != nil {
			err = os.NewSyscallError("io_uring_prep_sendmsg_zc", err)
			cb(Userdata{}, err)
		}
		return
	}
	if cqeFlags&cqeFMore != 0 {
		op.callback = nil
		bLen := 0
		if msg.Iov != nil {
			bLen = int(msg.Iov.Len)
		}
		oobn := int(msg.Controllen)
		if oobn > 0 && bLen == 0 {
			result = 0
		}
		cb(Userdata{N: result, OOBN: oobn}, nil)
		return
	}
	if cqeFlags&cqeFNotification != 0 {
		releaseOperator(op)
		return
	}
	releaseOperator(op)
	if cb != nil {
		err = os.NewSyscallError("io_uring_prep_send_zc", errors.New("invalid cqe flags"))
		cb(Userdata{}, err)
	}
	return
}
