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
	op := fd.WriteOperator()

	// msg
	bufAddr := uintptr(unsafe.Pointer(&b[0]))
	bufLen := uint32(len(b))

	// cb
	op.callback = cb
	// completion
	op.completion = completeSend

	// cylinder
	cylinder := nextIOURingCylinder()
	op.setCylinder(cylinder)

	// prepare
	err := cylinder.prepareRW(opSendCode, fd.Fd(), bufAddr, bufLen, 0, 0, op.ptr())
	if err != nil {
		cb(Userdata{}, os.NewSyscallError("io_uring_prep_send", err))
		op.reset()
		return
	}
	return
}

func completeSend(result int, op *Operator, err error) {
	if err != nil {
		if opSendCode == opSendZC {
			op.hijacked = false
			err = os.NewSyscallError("io_uring_prep_send_zc", err)
		} else {
			err = os.NewSyscallError("io_uring_prep_send", err)
		}
		op.callback(Userdata{}, err)
		return
	}
	// zc
	if opSendCode == opSendZC {
		if op.cqeFlags&cqeFMore != 0 {
			op.hijacked = true
			op.n = uint32(result)
			return
		}
		if op.cqeFlags&cqeFNotification != 0 {
			op.hijacked = false
			op.callback(Userdata{N: int(op.n)}, nil)
		}
		return
	}
	// non_zc
	op.callback(Userdata{N: result}, nil)
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
	op := fd.WriteOperator()
	// msg
	op.msg = &syscall.Msghdr{
		Name:       (*byte)(unsafe.Pointer(rsa)),
		Namelen:    uint32(rsaLen),
		Pad_cgo_0:  [4]byte{},
		Iov:        nil,
		Iovlen:     0,
		Control:    nil,
		Controllen: 0,
		Flags:      0,
		Pad_cgo_1:  [4]byte{},
	}
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

	// cb
	op.callback = cb
	// completion
	op.completion = completeSendMsg
	// cylinder
	cylinder := nextIOURingCylinder()
	op.setCylinder(cylinder)

	// prepare
	err := cylinder.prepareRW(opSendMsgCode, fd.Fd(), uintptr(unsafe.Pointer(op.msg)), 1, 0, 0, op.ptr())
	if err != nil {
		cb(Userdata{}, os.NewSyscallError("io_uring_prep_sendmsg", err))
		op.reset()
		return
	}
	return
}

func completeSendMsg(result int, op *Operator, err error) {
	if err != nil {
		if opSendMsgCode == opSendMsgZC {
			op.hijacked = false
			err = os.NewSyscallError("io_uring_prep_sendmsg_zc", err)
		} else {
			err = os.NewSyscallError("io_uring_prep_sendmsg", err)
		}
		op.callback(Userdata{}, err)
		return
	}
	// zc
	if opSendMsgCode == opSendMsgZC {
		if op.cqeFlags&cqeFMore != 0 {
			op.hijacked = true
			op.n = uint32(result)
			return
		}
		if op.cqeFlags&cqeFNotification != 0 {
			op.hijacked = false
			n := int(op.n)
			oobn := int(op.msg.Controllen)
			bLen := 0
			if op.msg.Iov != nil {
				bLen = int(op.msg.Iov.Len)
			}
			if oobn > 0 && bLen == 0 {
				n = 0
			}
			op.callback(Userdata{N: n, OOBN: oobn}, nil)
		}
		return
	}
	// non_zc
	n := result
	bLen := 0
	if op.msg.Iov != nil {
		bLen = int(op.msg.Iov.Len)
	}
	oobn := int(op.msg.Controllen)
	if oobn > 0 && bLen == 0 {
		n = 0
	}
	op.callback(Userdata{N: n, OOBN: oobn}, nil)
	return
}
