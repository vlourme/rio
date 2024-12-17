//go:build windows

package aio

import (
	"errors"
	"syscall"
)

func Close(fd Fd, cb OperationCallback) {
	switch fd.(type) {
	case NetFd:
		handle := fd.Fd()
		err := syscall.Closesocket(syscall.Handle(handle))
		if err != nil {
			err = errors.Join(errors.New("aio.Operator: close failed"), err)
		}
		cb(handle, fd.WriteOperator().userdata, err)
		return
	case FileFd:
		handle := fd.Fd()
		err := syscall.Close(syscall.Handle(handle))
		if err != nil {
			err = errors.Join(errors.New("aio.Operator: close failed"), err)
		}
		cb(handle, fd.WriteOperator().userdata, err)
		return
	default:
		cb(0, fd.WriteOperator().userdata, errors.New("aio.Operator: close was not supported"))
		return
	}
}

func CloseImmediately(fd Fd) {
	Close(fd, func(result int, userdata Userdata, err error) {})
}
