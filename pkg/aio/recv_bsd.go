//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

import (
	"errors"
	"io"
	"runtime"
	"syscall"
)

func Recv(fd NetFd, b []byte, cb OperationCallback) {
	// op
	op := fd.ReadOperator()
	// msg
	bLen := len(b)
	if bLen > MaxRW {
		b = b[:MaxRW]
	}
	op.b = b

	op.callback = cb
	op.completion = completeRecv

	cylinder := nextKqueueCylinder()
	op.setCylinder(cylinder)

	if err := cylinder.prepareRead(fd.Fd(), op); err != nil {
		cb(Userdata{}, err)
		// reset
		op.reset()
	}
	return
}

func completeRecv(result int, op *Operator, err error) {
	cb := op.callback
	if err != nil {
		if errors.Is(err, ErrClosed) && result == 0 {
			cb(Userdata{}, io.EOF)
			return
		}
		cb(Userdata{}, err)
		return
	}
	if result == 0 && op.fd.ZeroReadIsEOF() {
		cb(Userdata{}, io.EOF)
		return
	}

	fd := op.fd.Fd()
	b := op.b
	if result > len(b) {
		b = b[:result]
	}
	for {
		n, rErr := syscall.Read(fd, b)
		if rErr != nil {
			n = 0
			if errors.Is(rErr, syscall.EINTR) || errors.Is(rErr, syscall.EAGAIN) {
				continue
			}
			cb(Userdata{}, rErr)
			break
		}
		if n == 0 && op.fd.ZeroReadIsEOF() {
			cb(Userdata{}, io.EOF)
			break
		}
		cb(Userdata{N: n}, nil)
		break
	}
	runtime.KeepAlive(b)
	return
}

func RecvFrom(fd NetFd, b []byte, cb OperationCallback) {
	// op
	op := fd.ReadOperator()
	// msg
	bLen := len(b)
	if bLen > MaxRW {
		b = b[:MaxRW]
	}
	op.b = b

	op.callback = cb
	op.completion = completeRecvFrom

	cylinder := nextKqueueCylinder()
	op.setCylinder(cylinder)

	if err := cylinder.prepareRead(fd.Fd(), op); err != nil {
		cb(Userdata{}, err)
		// reset
		op.reset()
	}
	return
}

func completeRecvFrom(result int, op *Operator, err error) {
	cb := op.callback

	if err != nil {
		cb(Userdata{}, err)
		return
	}
	if result == 0 {
		cb(Userdata{}, nil)
		return
	}
	fd := op.fd.Fd()
	network := op.fd.(NetFd).Network()
	b := op.b
	if result > len(b) {
		b = b[:result]
	}

	for {
		n, sa, rErr := syscall.Recvfrom(fd, b, 0)
		if rErr != nil {
			if errors.Is(rErr, syscall.EINTR) || errors.Is(rErr, syscall.EAGAIN) {
				continue
			}
			cb(Userdata{}, rErr)
			break
		}
		addr := SockaddrToAddr(network, sa)
		cb(Userdata{N: n, Addr: addr}, nil)
		break
	}
	runtime.KeepAlive(b)
	return
}

func RecvMsg(fd NetFd, b []byte, oob []byte, cb OperationCallback) {
	// op
	op := fd.ReadOperator()
	// msg
	bLen := len(b)
	if bLen > MaxRW {
		b = b[:MaxRW]
	}
	op.b = b
	op.oob = oob

	op.callback = cb
	op.completion = completeRecvMsg

	cylinder := nextKqueueCylinder()
	op.setCylinder(cylinder)

	if err := cylinder.prepareRead(fd.Fd(), op); err != nil {
		cb(Userdata{}, err)
		// reset
		op.reset()
	}
	return
}

func completeRecvMsg(result int, op *Operator, err error) {
	cb := op.callback
	if err != nil {
		cb(Userdata{}, err)
		return
	}
	if result == 0 {
		cb(Userdata{}, nil)
		return
	}
	fd := op.fd.Fd()
	network := op.fd.(NetFd).Network()
	b := op.b
	if result > len(b) {
		b = b[:result]
	}
	oob := op.oob
	for {
		n, oobn, flags, sa, rErr := syscall.Recvmsg(fd, b, oob, 0)
		if rErr != nil {
			if errors.Is(rErr, syscall.EINTR) || errors.Is(rErr, syscall.EAGAIN) {
				continue
			}
			cb(Userdata{}, rErr)
			break
		}
		addr := SockaddrToAddr(network, sa)
		cb(Userdata{N: n, OOBN: oobn, Addr: addr, MessageFlags: flags}, nil)
		break
	}
	runtime.KeepAlive(b)
	return
}
