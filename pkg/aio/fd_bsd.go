//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

import (
	"errors"
	"syscall"
)

func Close(fd Fd, cb OperationCallback) {
	switch fd.(type) {
	case NetFd:
		handle := fd.Fd()
		err := syscall.Close(handle)
		if err != nil {
			err = errors.Join(errors.New("aio.Operator: close failed"), err)
		}
		cb(handle, fd.WriteOperator().userdata, err)
		return
	case FileFd:
		cb(0, fd.WriteOperator().userdata, errors.New("aio.Operator: close was not supported"))
		return
	default:
		cb(0, fd.WriteOperator().userdata, errors.New("aio.Operator: close was not supported"))
		return
	}
}
