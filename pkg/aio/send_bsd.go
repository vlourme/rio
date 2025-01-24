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
	op := writeOperator(fd)
	// check buf
	bLen := len(b)
	if bLen == 0 {
		cb(-1, Userdata{}, ErrEmptyBytes)
		return
	} else if bLen > MaxRW {
		b = b[:MaxRW]
	}
	// msg
	op.userdata.Msg.Append(b)

	// cb
	op.callback = cb
	// completion
	op.completion = func(result int, cop *Operator, err error) {
		completeSend(result, cop, err)
		runtime.KeepAlive(op)
	}

	// timeout
	if timeout := op.timeout; timeout > 0 {
		timer := getOperatorTimer()
		op.timer = timer
		timer.Start(timeout, &operatorCanceler{
			op: op,
		})
	}

	cylinder := nextKqueueCylinder()
	if err := cylinder.prepareWrite(fd.Fd(), op); err != nil {
		cb(-1, Userdata{}, err)
		// reset
		op.callback = nil
		op.completion = nil
		if timer := op.timer; timer != nil {
			timer.Done()
			putOperatorTimer(timer)
		}
	}
	return
}

func completeSend(result int, op *Operator, err error) {
	cb := op.callback
	userdata := op.userdata
	if err != nil || result == 0 {
		cb(0, userdata, err)
		return
	}

	fd := op.fd.Fd()
	b := userdata.Msg.Bytes(0)
	bLen := len(b)
	if bLen > result {
		b = b[:result]
		bLen = result
	}
	timer := op.timer
	for {
		if timer != nil && timer.DeadlineExceeded() {
			cb(-1, Userdata{}, ErrOperationDeadlineExceeded)
			break
		}
		n, wErr := syscall.Write(fd, b)
		if wErr != nil {
			n = 0
			if errors.Is(wErr, syscall.EINTR) || errors.Is(wErr, syscall.EAGAIN) {
				continue
			}
			cb(n, userdata, wErr)
			break
		}
		userdata.QTY = uint32(n)
		cb(n, userdata, nil)
		break
	}
	runtime.KeepAlive(userdata)
	return
}

func SendTo(fd NetFd, b []byte, addr net.Addr, cb OperationCallback) {
	// op
	op := writeOperator(fd)
	// check buf
	bLen := len(b)
	if bLen == 0 {
		cb(-1, Userdata{}, ErrEmptyBytes)
		return
	}
	if addr == nil {
		cb(-1, Userdata{}, ErrNilAddr)
		return
	}
	// msg
	op.userdata.Msg.Append(b)
	_, addrErr := op.userdata.Msg.SetAddr(addr)
	if addrErr != nil {
		cb(-1, Userdata{}, addrErr)
		return
	}

	// cb
	op.callback = cb
	// completion
	op.completion = func(result int, cop *Operator, err error) {
		completeSendTo(result, cop, err)
		runtime.KeepAlive(op)
	}

	// timeout
	if timeout := op.timeout; timeout > 0 {
		timer := getOperatorTimer()
		op.timer = timer
		timer.Start(timeout, &operatorCanceler{
			op: op,
		})
	}

	cylinder := nextKqueueCylinder()
	if err := cylinder.prepareWrite(fd.Fd(), op); err != nil {
		cb(-1, Userdata{}, err)
		// reset
		op.callback = nil
		op.completion = nil
		if timer := op.timer; timer != nil {
			timer.Done()
			putOperatorTimer(timer)
		}
	}
	return
}

func completeSendTo(result int, op *Operator, err error) {
	cb := op.callback
	userdata := op.userdata
	if err != nil {
		cb(result, userdata, err)
		return
	}
	if result == 0 {
		cb(0, userdata, nil)
		return
	}

	fd := op.fd.Fd()
	b := userdata.Msg.Bytes(0)
	bLen := len(b)
	if bLen > result {
		b = b[:result]
		bLen = result
	}
	flags := int(userdata.Msg.Flags())
	addr, addrErr := userdata.Msg.Addr()
	if addrErr != nil {
		cb(0, userdata, addrErr)
		return
	}
	sa := AddrToSockaddr(addr)

	timer := op.timer
	for {
		if timer != nil && timer.DeadlineExceeded() {
			cb(-1, Userdata{}, ErrOperationDeadlineExceeded)
			break
		}
		wErr := syscall.Sendto(fd, b, flags, sa)
		if wErr != nil {
			if errors.Is(wErr, syscall.EINTR) || errors.Is(wErr, syscall.EAGAIN) {
				continue
			}
			cb(-1, Userdata{}, wErr)
			break
		}
		userdata.QTY = uint32(bLen)
		cb(bLen, userdata, nil)
		break
	}
	runtime.KeepAlive(userdata)
	return
}

func SendMsg(fd NetFd, b []byte, oob []byte, addr net.Addr, cb OperationCallback) {
	// op
	op := writeOperator(fd)
	// check buf
	bLen := len(b)
	if bLen == 0 {
		cb(-1, Userdata{}, ErrEmptyBytes)
		return
	}
	if addr == nil {
		cb(-1, Userdata{}, ErrNilAddr)
		return
	}
	// msg
	op.userdata.Msg.Append(b)
	op.userdata.Msg.SetControl(oob)
	_, addrErr := op.userdata.Msg.SetAddr(addr)
	if addrErr != nil {
		cb(-1, Userdata{}, addrErr)
		return
	}

	// cb
	op.callback = cb
	// completion
	op.completion = func(result int, cop *Operator, err error) {
		completeSendMsg(result, cop, err)
		runtime.KeepAlive(op)
	}

	// timeout
	if timeout := op.timeout; timeout > 0 {
		timer := getOperatorTimer()
		op.timer = timer
		timer.Start(timeout, &operatorCanceler{
			op: op,
		})
	}

	cylinder := nextKqueueCylinder()
	if err := cylinder.prepareWrite(fd.Fd(), op); err != nil {
		cb(-1, Userdata{}, err)
		// reset
		op.callback = nil
		op.completion = nil
		if timer := op.timer; timer != nil {
			timer.Done()
			putOperatorTimer(timer)
		}
	}
	return
}

func completeSendMsg(result int, op *Operator, err error) {
	cb := op.callback
	userdata := op.userdata
	if err != nil {
		cb(result, userdata, err)
		return
	}
	if result == 0 {
		cb(0, userdata, nil)
		return
	}

	fd := op.fd.Fd()
	b := userdata.Msg.Bytes(0)
	bLen := len(b)
	if bLen > result {
		b = b[:result]
		bLen = result
	}
	oob := userdata.Msg.ControlBytes()
	flags := int(userdata.Msg.Flags())
	addr, addrErr := userdata.Msg.Addr()
	if addrErr != nil {
		cb(-1, Userdata{}, addrErr)
		return
	}
	sa := AddrToSockaddr(addr)

	timer := op.timer
	for {
		if timer != nil && timer.DeadlineExceeded() {
			cb(-1, Userdata{}, ErrOperationDeadlineExceeded)
			break
		}
		wErr := syscall.Sendmsg(fd, b, oob, sa, flags)
		if wErr != nil {
			if errors.Is(wErr, syscall.EINTR) || errors.Is(wErr, syscall.EAGAIN) {
				continue
			}
			cb(-1, Userdata{}, wErr)
			break
		}
		userdata.QTY = uint32(bLen)
		cb(bLen, userdata, nil)
		break
	}
	runtime.KeepAlive(userdata)
	return
}
