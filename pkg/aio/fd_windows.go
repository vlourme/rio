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
			cb(Userdata{}, err)
			return
		}
		cb(Userdata{}, nil)
		return
	case FileFd:
		handle := fd.Fd()
		err := syscall.Close(syscall.Handle(handle))
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
	Close(fd, func(userdata Userdata, err error) {})
}
