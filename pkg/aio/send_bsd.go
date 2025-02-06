//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

import (
	"errors"
	"net"
	"runtime"
	"syscall"
)

func Send(fd NetFd, b []byte, cb OperationCallback) {
	// op
	op := acquireOperator(fd)
	// msg
	op.b = b

	// cb
	op.callback = cb
	// completion
	op.completion = completeSend

	cylinder := nextKqueueCylinder()
	op.setCylinder(cylinder)

	if err := cylinder.prepareWrite(fd.Fd(), op); err != nil {
		cb(Userdata{}, err)
		releaseOperator(op)
	}
	return
}

func completeSend(result int, op *Operator, err error) {
	cb := op.callback
	fd := op.fd
	b := op.b

	releaseOperator(op)

	if err != nil {
		cb(Userdata{}, err)
		return
	}
	if result == 0 {
		cb(Userdata{}, nil)
		return
	}

	if len(b) > result {
		b = b[:result]
	}
	for {
		n, wErr := syscall.Write(fd.Fd(), b)
		if wErr != nil {
			n = 0
			if errors.Is(wErr, syscall.EINTR) || errors.Is(wErr, syscall.EAGAIN) {
				continue
			}
			cb(Userdata{}, wErr)
			break
		}
		cb(Userdata{N: n}, nil)
		break
	}

	runtime.KeepAlive(b)
	return
}

func SendTo(fd NetFd, b []byte, addr net.Addr, cb OperationCallback) {
	// op
	op := acquireOperator(fd)
	// msg
	op.b = b
	op.sa = AddrToSockaddr(addr)

	// cb
	op.callback = cb
	// completion
	op.completion = completeSendTo

	cylinder := nextKqueueCylinder()
	op.setCylinder(cylinder)

	if err := cylinder.prepareWrite(fd.Fd(), op); err != nil {
		cb(Userdata{}, err)
		releaseOperator(op)
	}
	return
}

func completeSendTo(result int, op *Operator, err error) {
	cb := op.callback
	fd := op.fd
	b := op.b
	sa := op.sa

	releaseOperator(op)

	if err != nil {
		cb(Userdata{}, err)
		return
	}
	if result == 0 {
		cb(Userdata{}, nil)
		return
	}

	bLen := len(b)
	if bLen > result {
		b = b[:result]
		bLen = result
	}
	flags := 0
	for {
		wErr := syscall.Sendto(fd.Fd(), b, flags, sa)
		if wErr != nil {
			if errors.Is(wErr, syscall.EINTR) || errors.Is(wErr, syscall.EAGAIN) {
				continue
			}
			cb(Userdata{}, wErr)
			break
		}
		cb(Userdata{N: bLen}, nil)
		break
	}
	runtime.KeepAlive(b)
	return
}

func SendMsg(fd NetFd, b []byte, oob []byte, addr net.Addr, cb OperationCallback) {
	// op
	op := acquireOperator(fd)
	// msg
	op.b = b
	op.oob = oob
	op.sa = AddrToSockaddr(addr)

	// cb
	op.callback = cb
	// completion
	op.completion = completeSendMsg

	cylinder := nextKqueueCylinder()
	op.setCylinder(cylinder)

	if err := cylinder.prepareWrite(fd.Fd(), op); err != nil {
		cb(Userdata{}, err)
		releaseOperator(op)
	}
	return
}

func completeSendMsg(result int, op *Operator, err error) {
	cb := op.callback

	fd := op.fd.Fd()
	b := op.b
	oob := op.oob
	sa := op.sa

	releaseOperator(op)

	if err != nil {
		cb(Userdata{}, err)
		return
	}
	if result == 0 {
		cb(Userdata{}, nil)
		return
	}

	bLen := len(b)
	if bLen > result {
		b = b[:result]
		bLen = result
	}

	flags := 0

	for {
		wErr := syscall.Sendmsg(fd, b, oob, sa, flags)
		if wErr != nil {
			if errors.Is(wErr, syscall.EINTR) || errors.Is(wErr, syscall.EAGAIN) {
				continue
			}
			cb(Userdata{}, wErr)
			break
		}
		cb(Userdata{N: bLen}, nil)
		break
	}
	runtime.KeepAlive(b)
	return
}
