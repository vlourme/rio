//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package aio

import (
	"github.com/brickingsoft/errors"
	"os"
	"syscall"
)

func Close(fd Fd, cb OperationCallback) {
	switch fd.(type) {
	case NetFd:
		handle := fd.Fd()
		err := syscall.Close(handle)
		if err != nil {
			err = errors.New(
				"close failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithMeta(errMetaOpKey, errMetaOpClose),
				errors.WithWrap(os.NewSyscallError("close", err)),
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
	_ = syscall.Close(fd.Fd())
}
