//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

import (
	"errors"
	"syscall"
)

func Close(fd Fd, cb OperationCallback) {
	op := WriteOperator(fd)
	op.userdata.Fd = fd
	switch fd.(type) {
	case NetFd:
		handle := fd.Fd()
		err := syscall.Close(handle)
		if err != nil {
			cb(-1, Userdata{}, err)
			return
		}
		cb(handle, op.userdata, err)
		return
	case FileFd:
		cb(-1, Userdata{}, errors.New("aio.Operator: close was not supported"))
		return
	default:
		cb(-1, Userdata{}, errors.New("aio.Operator: close was not supported"))
		return
	}
}

func CloseImmediately(fd Fd) {
	_ = syscall.Close(fd.Fd())
}
