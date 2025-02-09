//go:build windows

package aio

import (
	"github.com/brickingsoft/errors"
	"syscall"
)

func Close(fd Fd) (err error) {
	switch fd.(type) {
	case NetFd:
		handle := fd.Fd()
		err = syscall.Closesocket(syscall.Handle(handle))
		if err != nil {
			err = errors.New(
				"close failed",
				errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
				errors.WithMeta(errMetaOpKey, errMetaOpClose),
				errors.WithWrap(err),
			)
			return
		}
		return
	default:
		err = errors.New(
			"close failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpClose),
			errors.WithWrap(errors.Define("invalid fd")),
		)
		return
	}
}
