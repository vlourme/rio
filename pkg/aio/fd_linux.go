//go:build linux

package aio

import (
	"errors"
	"runtime"
	"syscall"
	"time"
)

func Close(fd Fd, cb OperationCallback) {
	op := fd.WriteOperator()
	ok := false
	for i := 0; i < 10; i++ {
		if op.hijacked.CompareAndSwap(false, true) {
			op.callback = cb
			op.completion = completeClose
			cylinder := nextIOURingCylinder()
			op.setCylinder(cylinder)
			err := cylinder.prepareRW(opClose, fd.Fd(), 0, 0, 0, 0, op.ptr())
			if err != nil {
				cb(Userdata{}, err)
				op.hijacked.Store(false)
				op.reset()
				break
			}
			ok = true
			break
		}
		const (
			waits = time.Nanosecond * 500
		)
		time.Sleep(waits)
	}
	if !ok {
		CloseImmediately(fd)
	}
	runtime.KeepAlive(op)
}

func completeClose(_ int, op *Operator, err error) {
	op.hijacked.Store(false)
	if err != nil {
		err = errors.Join(errors.New("aio.Operator: close failed"), err)
		op.callback(Userdata{}, err)
		return
	}
	op.callback(Userdata{}, nil)
	return
}

func CloseImmediately(fd Fd) {
	_ = syscall.Close(fd.Fd())
}
