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
	op := fd.WriteOperator()
	if opSendCode == opSend {
		bufAddr := uintptr(unsafe.Pointer(&b[0]))
		bufLen := uint32(bLen)
		op.b = b
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
	// send_zc
	if op.hijacked.CompareAndSwap(false, true) {
		bufAddr := uintptr(unsafe.Pointer(&b[0]))
		bufLen := uint32(bLen)
		op.b = b
		// cb
		op.callback = cb
		// completion
		op.completion = completeSendZC

		// cylinder
		cylinder := nextIOURingCylinder()
		op.setCylinder(cylinder)

		// prepare
		err := cylinder.prepareRW(opSendCode, fd.Fd(), bufAddr, bufLen, 0, 0, op.ptr())
		if err != nil {
			cb(Userdata{}, os.NewSyscallError("io_uring_prep_send_zc", err))
			op.reset()
			return
		}
	} else {
		op.sch <- sendZCRequest{
			b:        b,
			oob:      nil,
			addr:     nil,
			callback: cb,
		}
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

func completeSendZC(result int, op *Operator, err error) {
	if err != nil {
		err = os.NewSyscallError("io_uring_prep_send_zc", err)
		op.callback(Userdata{}, err)
		select {
		case r := <-op.sch:
			fd := op.fd.(NetFd)
			op.hijacked.Store(false)
			Send(fd, r.b, r.callback)
			break
		default:
			op.hijacked.Store(false)
			break
		}
		return
	}
	if op.cqeFlags&cqeFMore != 0 {
		op.callback(Userdata{N: result}, nil)
		op.callback = nil
		return
	}
	if op.cqeFlags&cqeFNotification != 0 {
		select {
		case r := <-op.sch:
			fd := op.fd.(NetFd)
			op.hijacked.Store(false)
			Send(fd, r.b, r.callback)
			break
		default:
			op.hijacked.Store(false)
			break
		}
		return
	}
	return
}

func SendTo(fd NetFd, b []byte, addr net.Addr, cb OperationCallback) {
	SendMsg(fd, b, nil, addr, cb)
}

var opSendMsgCode uint8 = 0

func newMsghdr(fd NetFd, b []byte, oob []byte, rsa *syscall.RawSockaddrAny, rsaLen uint32) *syscall.Msghdr {
	msg := &syscall.Msghdr{
		Name:       (*byte)(unsafe.Pointer(rsa)),
		Namelen:    rsaLen,
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
		msg.Iov = &syscall.Iovec{
			Base: &b[0],
			Len:  uint64(bLen),
		}
		msg.Iovlen = 1
	}
	if oobLen := len(oob); oobLen > 0 {
		msg.Control = &oob[0]
		msg.Controllen = uint64(oobLen)
		if bLen == 0 && fd.SocketType() != syscall.SOCK_DGRAM {
			var dummy byte
			msg.Iov.Base = &dummy
			msg.Iov.Len = uint64(1)
			msg.Iovlen = 1
		}
	}
	return msg
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
	if opSendMsgCode == opSendmsg {
		op.msg = newMsghdr(fd, b, oob, rsa, uint32(rsaLen))
		op.b = b
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
	}
	if op.hijacked.CompareAndSwap(false, true) {
		op.msg = newMsghdr(fd, b, oob, rsa, uint32(rsaLen))
		op.b = b
		// cb
		op.callback = cb
		// completion
		op.completion = completeSendMsgZC
		// cylinder
		cylinder := nextIOURingCylinder()
		op.setCylinder(cylinder)

		// prepare
		err := cylinder.prepareRW(opSendMsgCode, fd.Fd(), uintptr(unsafe.Pointer(op.msg)), 1, 0, 0, op.ptr())
		if err != nil {
			cb(Userdata{}, os.NewSyscallError("io_uring_prep_sendmsg_zc", err))
			op.reset()
			return
		}
	} else {
		op.sch <- sendZCRequest{
			b:        b,
			oob:      oob,
			addr:     addr,
			callback: cb,
		}
	}
	return
}

func completeSendMsg(result int, op *Operator, err error) {
	if err != nil {
		err = os.NewSyscallError("io_uring_prep_sendmsg", err)
		op.callback(Userdata{}, err)
		return
	}
	bLen := 0
	if op.msg.Iov != nil {
		bLen = int(op.msg.Iov.Len)
	}
	oobn := int(op.msg.Controllen)
	if oobn > 0 && bLen == 0 {
		result = 0
	}
	op.callback(Userdata{N: result, OOBN: oobn}, nil)
	return
}

func completeSendMsgZC(result int, op *Operator, err error) {
	if err != nil {
		err = os.NewSyscallError("io_uring_prep_sendmsg_zc", err)
		op.callback(Userdata{}, err)
		select {
		case r := <-op.sch:
			fd := op.fd.(NetFd)
			op.hijacked.Store(false)
			SendMsg(fd, r.b, r.oob, r.addr, r.callback)
			break
		default:
			op.hijacked.Store(false)
			break
		}
		return
	}
	if op.cqeFlags&cqeFMore != 0 {
		bLen := 0
		if op.msg.Iov != nil {
			bLen = int(op.msg.Iov.Len)
		}
		oobn := int(op.msg.Controllen)
		if oobn > 0 && bLen == 0 {
			result = 0
		}
		op.callback(Userdata{N: result, OOBN: oobn}, nil)
		op.callback = nil
		return
	}
	if op.cqeFlags&cqeFNotification != 0 {
		select {
		case r := <-op.sch:
			fd := op.fd.(NetFd)
			op.hijacked.Store(false)
			SendMsg(fd, r.b, r.oob, r.addr, r.callback)
			break
		default:
			op.hijacked.Store(false)
			break
		}
		return
	}
	return
}
