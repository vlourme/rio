//go:build linux

package aio

import (
	"errors"
	"runtime"
	"syscall"
)

func Close(fd Fd, cb OperationCallback) {
	op := writeOperator(fd)

	op.callback = cb
	op.completion = func(result int, cop *Operator, err error) {
		completeClose(result, cop, err)
		runtime.KeepAlive(op)
	}

	err := prepare(opClose, fd.Fd(), 0, 0, 0, 0, op)
	runtime.KeepAlive(op)
	if err != nil {
		cb(-1, Userdata{}, err)
		// reset
		op.completion = nil
		op.callback = nil
	}
}

func completeClose(result int, op *Operator, err error) {
	if err != nil {
		err = errors.Join(errors.New("aio.Operator: close failed"), err)
		op.callback(-1, Userdata{}, err)
		return
	}
	op.callback(result, op.userdata, nil)
	return
}

func CloseImmediately(fd Fd) {
	_ = syscall.Close(fd.Fd())
}
