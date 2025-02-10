//go:build linux

package aio

import (
	"github.com/brickingsoft/errors"
	"syscall"
)

func Close(fd Fd) (err error) {
	sock := fd.Fd()
	if err = syscall.Close(sock); err != nil {
		err = errors.New(
			"close failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpClose),
			errors.WithWrap(err),
		)
		return
	}
	return
}
