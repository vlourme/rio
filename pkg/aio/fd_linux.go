//go:build linux

package aio

import (
	"errors"
	"unsafe"
)

func Close(fd Fd, cb OperationCallback) {
	op := fd.WriteOperator()

	op.callback = cb
	op.completion = completeClose

	userdata := uint64(uintptr(unsafe.Pointer(&op)))

	err := prepare(opClose, fd.Fd(), 0, 0, 0, 0, userdata)
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
