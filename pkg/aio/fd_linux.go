//go:build linux

package aio

import (
	"errors"
	"runtime"
)

func Close(fd Fd, cb OperationCallback) {
	op := fd.WriteOperator()

	op.callback = cb
	op.completion = completeClose

	err := prepare(opClose, fd.Fd(), 0, 0, 0, 0, &op)
	runtime.KeepAlive(&op)
	if err != nil {
		cb(0, op.userdata, err)
		// reset
		op.completion = nil
		op.callback = nil
	}
}

func completeClose(result int, op *Operator, err error) {
	if err != nil {
		err = errors.Join(errors.New("aio.Operator: close failed"), err)
		op.callback(result, op.userdata, err)
		return
	}
	op.callback(result, op.userdata, nil)
	return
}
