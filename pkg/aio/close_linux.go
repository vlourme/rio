//go:build linux

package aio

import (
	"github.com/brickingsoft/errors"
	"os"
	"syscall"
)

func Close(fd Fd, cb OperationCallback) {
	op := acquireOperator(fd)
	op.callback = cb
	op.completion = completeClose
	cylinder := nextIOURingCylinder()
	op.setCylinder(cylinder)
	err := cylinder.prepareRW(opClose, fd.Fd(), 0, 0, 0, 0, op.ptr())
	if err != nil {
		CloseImmediately(fd)
		releaseOperator(op)
		err = errors.New(
			"close failed",
			errors.WithMeta(errMetaPkgKey, errMetaPkgVal),
			errors.WithMeta(errMetaOpKey, errMetaOpClose),
			errors.WithWrap(os.NewSyscallError("io_uring_prep_close", err)),
		)
		cb(Userdata{}, err)
	}
}

func completeClose(_ int, op *Operator, err error) {
	cb := op.callback
	releaseOperator(op)
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
}

func CloseImmediately(fd Fd) {
	_ = syscall.Close(fd.Fd())
}
