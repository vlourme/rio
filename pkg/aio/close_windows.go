//go:build windows

package aio

import (
	"github.com/brickingsoft/errors"
	"syscall"
)

func Close(fd Fd, cb OperationCallback) {
	switch fd.(type) {
	case NetFd:
		handle := fd.Fd()
		err := syscall.Closesocket(syscall.Handle(handle))
		if err != nil {
			err = errors.New(
				"close failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithMeta(errMetaOpKey, errMetaOpClose),
				errors.WithWrap(err),
			)
			cb(Userdata{}, err)
			return
		}
		cb(Userdata{}, nil)
		return
	default:
		err := errors.New(
			"close failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpClose),
			errors.WithWrap(errors.Define("invalid fd")),
		)
		cb(Userdata{}, err)
		return
	}
}

func CloseImmediately(fd Fd) {
	Close(fd, func(userdata Userdata, err error) {})
}
