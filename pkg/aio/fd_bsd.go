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
			cb(Userdata{}, err)
			return
		}
		cb(Userdata{}, nil)
		return
	default:
		cb(Userdata{}, errors.New("aio.Operator: close was not supported"))
		return
	}
}

func CloseImmediately(fd Fd) {
	_ = syscall.Close(fd.Fd())
}
