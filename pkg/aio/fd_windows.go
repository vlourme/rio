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
			cb(-1, Userdata{}, err)
			return
		}
		cb(handle, Userdata{}, nil)
		return
	case FileFd:
		handle := fd.Fd()
		err := syscall.Close(syscall.Handle(handle))
		if err != nil {
			cb(-1, Userdata{}, err)
			return
		}
		cb(handle, Userdata{}, nil)
		return
	default:
		cb(-1, Userdata{}, errors.New("aio.Operator: close was not supported"))
		return
	}
}

func CloseImmediately(fd Fd) {
	Close(fd, func(result int, userdata Userdata, err error) {})
}
